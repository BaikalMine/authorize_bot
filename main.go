package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"autorize-bot-tg/internal/captcha"
	"autorize-bot-tg/internal/config"
)

const callbackPrefix = "captcha"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	bot, err := newBotWithRetry(cfg)
	if err != nil {
		log.Fatalf("telegram init error: %v", redactToken(err.Error(), cfg.BotToken))
	}

	log.Printf("authorized as @%s", bot.Self.UserName)

	store := captcha.NewStore()
	go cleanupExpired(bot, store, cfg)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = cfg.PollingTimeout
	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, store, cfg, update.Message)
			continue
		}
		if update.CallbackQuery != nil {
			handleCallback(bot, store, update.CallbackQuery)
		}
	}
}

func newBotWithRetry(cfg config.Config) (*tgbotapi.BotAPI, error) {
	var lastErr error
	attempts := max(1, cfg.StartupRetries+1)

	for attempt := 1; attempt <= attempts; attempt++ {
		bot, err := newBot(cfg)
		if err == nil {
			return bot, nil
		}

		lastErr = err
		if attempt == 1 && cfg.NetworkDiagnostics {
			logTelegramNetworkDiagnostics(cfg)
		}
		if attempt == attempts {
			break
		}

		log.Printf(
			"telegram init failed, retrying in %s: %s",
			cfg.StartupRetryDelay,
			redactToken(err.Error(), cfg.BotToken),
		)
		time.Sleep(cfg.StartupRetryDelay)
	}

	return nil, lastErr
}

func newBot(cfg config.Config) (*tgbotapi.BotAPI, error) {
	endpoint := tgbotapi.APIEndpoint
	if cfg.TelegramAPIEndpoint != "" {
		endpoint = cfg.TelegramAPIEndpoint
	}

	return tgbotapi.NewBotAPIWithClient(cfg.BotToken, endpoint, telegramHTTPClient(cfg))
}

func telegramHTTPClient(cfg config.Config) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   cfg.TelegramConnectTimeout,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, cfg.TelegramIPFamily, address)
		},
		TLSHandshakeTimeout:   cfg.TelegramConnectTimeout,
		ResponseHeaderTimeout: cfg.TelegramRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.TelegramRequestTimeout,
	}
}

func logTelegramNetworkDiagnostics(cfg config.Config) {
	endpoint := cfg.TelegramAPIEndpoint
	if endpoint == "" {
		endpoint = tgbotapi.APIEndpoint
	}

	host, port := endpointHostPort(endpoint)
	if host == "" {
		log.Printf("telegram network diagnostics: cannot parse endpoint host from %q", endpoint)
		return
	}

	log.Printf("telegram network diagnostics: endpoint_host=%s endpoint_port=%s ip_family=%s proxy_env=%s", host, port, cfg.TelegramIPFamily, proxyEnvSummary())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TelegramConnectTimeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		log.Printf("telegram network diagnostics: dns lookup failed: %v", err)
	} else {
		log.Printf("telegram network diagnostics: dns addresses=%s", formatIPAddrs(addrs))
	}

	conn, err := (&net.Dialer{Timeout: cfg.TelegramConnectTimeout}).DialContext(ctx, cfg.TelegramIPFamily, net.JoinHostPort(host, port))
	if err != nil {
		log.Printf("telegram network diagnostics: tcp connect failed: %v", err)
		return
	}
	_ = conn.Close()
	log.Printf("telegram network diagnostics: tcp connect ok")
}

func endpointHostPort(endpoint string) (string, string) {
	rawURL := fmt.Sprintf(endpoint, "token", "getMe")
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}

	return parsed.Hostname(), port
}

func proxyEnvSummary() string {
	keys := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "ALL_PROXY", "all_proxy"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		state := "empty"
		if os.Getenv(key) != "" {
			state = "set"
		}
		parts = append(parts, key+"="+state)
	}
	return strings.Join(parts, " ")
}

func formatIPAddrs(addrs []net.IPAddr) string {
	values := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		values = append(values, addr.String())
	}
	return strings.Join(values, ",")
}

func handleMessage(bot *tgbotapi.BotAPI, store *captcha.Store, cfg config.Config, message *tgbotapi.Message) {
	if len(message.NewChatMembers) == 0 {
		return
	}

	for _, user := range message.NewChatMembers {
		if user.ID == bot.Self.ID || user.IsBot {
			continue
		}

		if err := restrictUser(bot, message.Chat.ID, user.ID); err != nil {
			log.Printf("restrict user %d in chat %d: %v", user.ID, message.Chat.ID, err)
		}

		challenge, err := store.Create(message.Chat.ID, user.ID, cfg.CaptchaTimeout)
		if err != nil {
			log.Printf("create captcha for user %d: %v", user.ID, err)
			continue
		}

		sent, err := bot.Send(captchaMessage(message.Chat.ID, user, challenge, cfg.CaptchaTimeout))
		if err != nil {
			log.Printf("send captcha to user %d: %v", user.ID, err)
			continue
		}
		store.SetMessageID(message.Chat.ID, user.ID, sent.MessageID)
	}
}

func handleCallback(bot *tgbotapi.BotAPI, store *captcha.Store, query *tgbotapi.CallbackQuery) {
	chatID, userID, answer, ok := parseCallback(query.Data)
	if !ok {
		return
	}

	if query.From == nil || int64(query.From.ID) != userID {
		answerCallback(bot, query.ID, "Это капча другого участника.")
		return
	}

	challenge, ok := store.Get(chatID, userID)
	if !ok {
		answerCallback(bot, query.ID, "Проверка уже истекла.")
		return
	}

	if answer != challenge.Answer {
		answerCallback(bot, query.ID, "Неверно. Попробуйте еще раз.")
		return
	}

	if err := unrestrictUser(bot, chatID, userID); err != nil {
		log.Printf("unrestrict user %d in chat %d: %v", userID, chatID, err)
		answerCallback(bot, query.ID, "Ответ верный, но бот не смог вернуть права. Проверьте права администратора.")
		return
	}

	store.Delete(chatID, userID)
	answerCallback(bot, query.ID, "Готово, добро пожаловать!")

	edit := tgbotapi.NewEditMessageText(chatID, challenge.MessageID, fmt.Sprintf("%s прошел проверку.", userMention(query.From)))
	edit.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(edit)
}

func cleanupExpired(bot *tgbotapi.BotAPI, store *captcha.Store, cfg config.Config) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, challenge := range store.Expired(time.Now()) {
			if cfg.KickOnTimeout {
				if err := kickUser(bot, challenge.ChatID, challenge.UserID); err != nil {
					log.Printf("kick expired user %d in chat %d: %v", challenge.UserID, challenge.ChatID, err)
				}
			}
			if challenge.MessageID != 0 {
				text := "Время на прохождение капчи истекло."
				edit := tgbotapi.NewEditMessageText(challenge.ChatID, challenge.MessageID, text)
				_, _ = bot.Send(edit)
			}
		}
	}
}

func captchaMessage(chatID int64, user tgbotapi.User, challenge captcha.Challenge, timeout time.Duration) tgbotapi.MessageConfig {
	text := fmt.Sprintf(
		"%s, подтвердите, что вы человек.\n\nРешите пример: <b>%s</b>\nВремя: %s",
		userMention(&user),
		challenge.Question,
		timeout.Round(time.Second),
	)

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 2)
	for i := 0; i < len(challenge.Options); i += 2 {
		var row []tgbotapi.InlineKeyboardButton
		for _, option := range challenge.Options[i:min(i+2, len(challenge.Options))] {
			data := fmt.Sprintf("%s:%d:%d:%d", callbackPrefix, chatID, user.ID, option)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(strconv.Itoa(option), data))
		}
		rows = append(rows, row)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	return msg
}

func restrictUser(bot *tgbotapi.BotAPI, chatID int64, userID int64) error {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		UntilDate: time.Now().Add(24 * time.Hour).Unix(),
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       false,
			CanSendMediaMessages:  false,
			CanSendPolls:          false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
			CanChangeInfo:         false,
			CanInviteUsers:        false,
			CanPinMessages:        false,
		},
	}
	_, err := bot.Request(cfg)
	return err
}

func unrestrictUser(bot *tgbotapi.BotAPI, chatID int64, userID int64) error {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       true,
			CanSendMediaMessages:  true,
			CanSendPolls:          true,
			CanSendOtherMessages:  true,
			CanAddWebPagePreviews: true,
			CanChangeInfo:         false,
			CanInviteUsers:        true,
			CanPinMessages:        false,
		},
	}
	_, err := bot.Request(cfg)
	return err
}

func kickUser(bot *tgbotapi.BotAPI, chatID int64, userID int64) error {
	ban := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		UntilDate: time.Now().Add(30 * time.Second).Unix(),
	}
	if _, err := bot.Request(ban); err != nil {
		return err
	}

	unban := tgbotapi.UnbanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		OnlyIfBanned: true,
	}
	_, err := bot.Request(unban)
	return err
}

func parseCallback(data string) (chatID int64, userID int64, answer int, ok bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 || parts[0] != callbackPrefix {
		return 0, 0, 0, false
	}

	parsedChatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	parsedUserID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	parsedAnswer, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, 0, false
	}
	return parsedChatID, parsedUserID, parsedAnswer, true
}

func answerCallback(bot *tgbotapi.BotAPI, queryID, text string) {
	callback := tgbotapi.NewCallback(queryID, text)
	_, _ = bot.Request(callback)
}

func userMention(user *tgbotapi.User) string {
	if user.UserName != "" {
		return "@" + user.UserName
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = "Пользователь"
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, user.ID, htmlEscape(name))
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func redactToken(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "<redacted>")
}
