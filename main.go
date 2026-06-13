package main

import (
	"context"
	"encoding/json"
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
	"autorize-bot-tg/internal/probation"
)

const callbackPrefix = "captcha"

type telegramClient interface {
	Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

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

	store := captcha.NewStore(captcha.Limits{
		MaxActive:        cfg.MaxActiveChallenges,
		MaxActivePerChat: cfg.MaxActiveChallengesPerChat,
	})
	probationStore := probation.NewStore()
	go cleanupExpired(bot, store, probationStore, cfg)

	for update := range pollUpdates(bot, cfg) {
		if update.Message != nil {
			handleMessage(bot, bot.Self.ID, store, probationStore, cfg, update.Message)
			continue
		}
		if update.CallbackQuery != nil {
			handleCallback(bot, store, probationStore, cfg, update.CallbackQuery)
		}
	}
}

func pollUpdates(bot *tgbotapi.BotAPI, cfg config.Config) <-chan tgbotapi.Update {
	updates := make(chan tgbotapi.Update, bot.Buffer)

	go func() {
		defer close(updates)

		updateConfig := tgbotapi.NewUpdate(0)
		updateConfig.Timeout = cfg.PollingTimeout

		for {
			batch, err := bot.GetUpdates(updateConfig)
			if err != nil {
				log.Printf("get updates failed: %s", safeTelegramError(err, cfg.BotToken))
				log.Printf("failed to get updates, retrying in 3 seconds")
				time.Sleep(3 * time.Second)
				continue
			}

			for _, update := range batch {
				if update.UpdateID >= updateConfig.Offset {
					updateConfig.Offset = update.UpdateID + 1
					updates <- update
				}
			}
		}
	}()

	return updates
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
			return dialTelegram(ctx, cfg, address)
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

func dialTelegram(ctx context.Context, cfg config.Config, address string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   cfg.TelegramConnectTimeout,
		KeepAlive: 30 * time.Second,
	}

	var lastErr error
	for _, network := range telegramNetworks(cfg) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func telegramNetworks(cfg config.Config) []string {
	if !cfg.TelegramIPFallback || cfg.TelegramIPFamily == "tcp" {
		return []string{cfg.TelegramIPFamily}
	}
	if cfg.TelegramIPFamily == "tcp6" {
		return []string{"tcp6", "tcp4"}
	}
	return []string{"tcp4", "tcp6"}
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

func handleMessage(bot telegramClient, botID int64, store *captcha.Store, probationStore *probation.Store, cfg config.Config, message *tgbotapi.Message) {
	logSuspiciousMessage(cfg, message)

	if len(message.NewChatMembers) == 0 {
		if handleProbationMessage(bot, probationStore, cfg, message) {
			return
		}
		handleGlobalSpamMessage(bot, cfg, message)
		return
	}
	if message.Chat == nil {
		return
	}

	for _, user := range message.NewChatMembers {
		if user.ID == botID || user.IsBot {
			continue
		}

		if err := restrictUser(bot, message.Chat.ID, user.ID); err != nil {
			log.Printf("restrict user %d in chat %d: %s", user.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
			if cfg.KickOnTimeout {
				if err := kickUser(bot, message.Chat.ID, user.ID); err != nil {
					log.Printf("kick unrestricted user %d in chat %d: %s", user.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
				}
			}
			continue
		}

		challenge, err := store.Create(message.Chat.ID, user.ID, cfg.CaptchaTimeout)
		if err != nil {
			log.Printf("create captcha for user %d: %v", user.ID, err)
			if cfg.KickOnTimeout {
				if err := kickUser(bot, message.Chat.ID, user.ID); err != nil {
					log.Printf("kick user without captcha %d in chat %d: %s", user.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
				}
			}
			continue
		}

		sent, err := bot.Send(captchaMessage(message.Chat.ID, user, challenge, cfg.CaptchaTimeout))
		if err != nil {
			log.Printf("send captcha to user %d: %s", user.ID, safeTelegramError(err, cfg.BotToken))
			store.Delete(message.Chat.ID, user.ID)
			if cfg.KickOnTimeout {
				if err := kickUser(bot, message.Chat.ID, user.ID); err != nil {
					log.Printf("kick user without captcha message %d in chat %d: %s", user.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
				}
			}
			continue
		}
		store.SetMessageID(message.Chat.ID, user.ID, sent.MessageID)
	}

	if message.MessageID != 0 {
		if err := deleteMessage(bot, message.Chat.ID, message.MessageID); err != nil {
			log.Printf("delete join service message %d in chat %d: %s", message.MessageID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
		}
	}
}

func handleCallback(bot telegramClient, store *captcha.Store, probationStore *probation.Store, cfg config.Config, query *tgbotapi.CallbackQuery) {
	chatID, userID, answer, ok := parseCallback(query.Data)
	if !ok {
		return
	}

	if query.From == nil || int64(query.From.ID) != userID {
		answerCallback(bot, query.ID, "Это капча другого участника.")
		return
	}

	challenge, ok, expired := store.GetValid(chatID, userID, time.Now())
	if !ok {
		answerCallback(bot, query.ID, "Проверка уже истекла.")
		if expired && cfg.KickOnTimeout {
			if err := kickUser(bot, chatID, userID); err != nil {
				log.Printf("kick expired callback user %d in chat %d: %s", userID, chatID, safeTelegramError(err, cfg.BotToken))
			}
		}
		return
	}

	if answer != challenge.Answer {
		remaining, locked := store.RecordFailedAttempt(chatID, userID, cfg.CaptchaMaxAttempts)
		if locked {
			answerCallback(bot, query.ID, "Слишком много неверных ответов.")
			if challenge.MessageID != 0 {
				if err := deleteMessage(bot, chatID, challenge.MessageID); err != nil {
					log.Printf("delete failed captcha message %d in chat %d: %s", challenge.MessageID, chatID, safeTelegramError(err, cfg.BotToken))
				}
			}
			if cfg.KickOnTimeout {
				if err := kickUser(bot, chatID, userID); err != nil {
					log.Printf("kick failed captcha user %d in chat %d: %s", userID, chatID, safeTelegramError(err, cfg.BotToken))
				}
			}
			return
		}
		if remaining > 0 {
			answerCallback(bot, query.ID, fmt.Sprintf("Неверно. Осталось попыток: %d.", remaining))
			return
		}
		answerCallback(bot, query.ID, "Неверно. Попробуйте еще раз.")
		return
	}

	if cfg.ProbationEnabled {
		if err := restrictUserForProbation(bot, chatID, userID, cfg.ProbationDuration); err != nil {
			log.Printf("probation restrict user %d in chat %d: %s", userID, chatID, safeTelegramError(err, cfg.BotToken))
			answerCallback(bot, query.ID, "Ответ верный, но бот не смог включить проверочный режим. Проверьте права администратора.")
			return
		}
		probationStore.Add(chatID, userID, cfg.ProbationDuration, time.Now())
	} else {
		if err := unrestrictUser(bot, chatID, userID); err != nil {
			log.Printf("unrestrict user %d in chat %d: %s", userID, chatID, safeTelegramError(err, cfg.BotToken))
			answerCallback(bot, query.ID, "Ответ верный, но бот не смог вернуть права. Проверьте права администратора.")
			return
		}
	}

	store.Delete(chatID, userID)
	answerCallback(bot, query.ID, "Готово, добро пожаловать!")
	if challenge.MessageID != 0 {
		if err := deleteMessage(bot, chatID, challenge.MessageID); err != nil {
			log.Printf("delete passed captcha message %d in chat %d: %s", challenge.MessageID, chatID, safeTelegramError(err, cfg.BotToken))
		}
	}
}

func handleProbationMessage(bot telegramClient, probationStore *probation.Store, cfg config.Config, message *tgbotapi.Message) bool {
	if !cfg.ProbationEnabled || message.Chat == nil || message.From == nil || message.From.IsBot {
		return false
	}
	if !probationStore.Active(message.Chat.ID, int64(message.From.ID), time.Now()) {
		return false
	}
	if !isProbationSpam(message, cfg) {
		return false
	}

	if err := deleteMessage(bot, message.Chat.ID, message.MessageID); err != nil {
		log.Printf("delete probation spam message %d in chat %d: %s", message.MessageID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
	}
	if cfg.KickOnTimeout {
		if err := kickUser(bot, message.Chat.ID, int64(message.From.ID)); err != nil {
			log.Printf("kick probation spam user %d in chat %d: %s", message.From.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
		}
	}
	probationStore.Delete(message.Chat.ID, int64(message.From.ID))
	return true
}

func handleGlobalSpamMessage(bot telegramClient, cfg config.Config, message *tgbotapi.Message) bool {
	if !cfg.SpamGuardEnabled || message.Chat == nil || message.From == nil || message.From.IsBot {
		return false
	}
	if !isKnownSpamMessage(message) {
		return false
	}

	if err := deleteMessage(bot, message.Chat.ID, message.MessageID); err != nil {
		log.Printf("delete global spam message %d in chat %d: %s", message.MessageID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
	}
	if cfg.SpamGuardKick {
		if err := kickUser(bot, message.Chat.ID, int64(message.From.ID)); err != nil {
			log.Printf("kick global spam user %d in chat %d: %s", message.From.ID, message.Chat.ID, safeTelegramError(err, cfg.BotToken))
		}
	}
	return true
}

func cleanupExpired(bot *tgbotapi.BotAPI, store *captcha.Store, probationStore *probation.Store, cfg config.Config) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, challenge := range store.Expired(time.Now(), cfg.CleanupBatchSize) {
			if cfg.KickOnTimeout {
				if err := kickUser(bot, challenge.ChatID, challenge.UserID); err != nil {
					log.Printf("kick expired user %d in chat %d: %s", challenge.UserID, challenge.ChatID, safeTelegramError(err, cfg.BotToken))
				}
			}
			if challenge.MessageID != 0 {
				if err := deleteMessage(bot, challenge.ChatID, challenge.MessageID); err != nil {
					log.Printf("delete expired captcha message %d in chat %d: %s", challenge.MessageID, challenge.ChatID, safeTelegramError(err, cfg.BotToken))
				}
			}
		}
		for _, entry := range probationStore.Expired(time.Now(), cfg.CleanupBatchSize) {
			if err := unrestrictUser(bot, entry.ChatID, entry.UserID); err != nil {
				log.Printf("unrestrict probation user %d in chat %d: %s", entry.UserID, entry.ChatID, safeTelegramError(err, cfg.BotToken))
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

func restrictUser(bot telegramClient, chatID int64, userID int64) error {
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

func unrestrictUser(bot telegramClient, chatID int64, userID int64) error {
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

func restrictUserForProbation(bot telegramClient, chatID int64, userID int64, duration time.Duration) error {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		UntilDate: time.Now().Add(duration).Unix(),
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       true,
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

func kickUser(bot telegramClient, chatID int64, userID int64) error {
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

func deleteMessage(bot telegramClient, chatID int64, messageID int) error {
	cfg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := bot.Request(cfg)
	return err
}

func isProbationSpam(message *tgbotapi.Message, cfg config.Config) bool {
	if cfg.ProbationBlockLinks && messageHasLink(message) {
		return true
	}
	if cfg.ProbationBlockForwards && messageIsForward(message) {
		return true
	}
	if cfg.ProbationBlockMedia && messageHasMedia(message) {
		return true
	}
	return false
}

func messageHasLink(message *tgbotapi.Message) bool {
	for _, entity := range append(message.Entities, message.CaptionEntities...) {
		if entity.Type == "url" || entity.Type == "text_link" {
			return true
		}
	}
	if messageHasInlineKeyboardURL(message) {
		return true
	}

	text := strings.ToLower(message.Text + " " + message.Caption)
	linkMarkers := []string{"http://", "https://", "www.", "t.me/", "telegram.me/", "bit.ly/", "tinyurl.com/"}
	for _, marker := range linkMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isKnownSpamMessage(message *tgbotapi.Message) bool {
	if !messageHasLink(message) {
		return false
	}

	text := spamText(message)
	markers := []string{
		"удален", "удалён", "удалёнка", "удаленка",
		"от 18", "в неделю", "пишите", "₽", "руб",
		"заработ", "подработ", "доход", "выплат",
		"платим", "билайн", "номер", "код", "кода",
		"забрать", "менеджер", "инструкц",
	}

	score := 0
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			score++
		}
	}
	if strings.Contains(text, "t.me/m/") && score >= 1 {
		return true
	}
	return score >= 2
}

func spamText(message *tgbotapi.Message) string {
	parts := []string{message.Text, message.Caption}
	for _, entity := range append(message.Entities, message.CaptionEntities...) {
		if entity.URL != "" {
			parts = append(parts, entity.URL)
		}
	}
	for _, row := range inlineKeyboardRows(message) {
		for _, button := range row {
			parts = append(parts, button.Text)
			if button.URL != nil {
				parts = append(parts, *button.URL)
			}
			if button.LoginURL != nil {
				parts = append(parts, button.LoginURL.URL)
			}
		}
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func messageHasInlineKeyboardURL(message *tgbotapi.Message) bool {
	for _, row := range inlineKeyboardRows(message) {
		for _, button := range row {
			if button.URL != nil && *button.URL != "" {
				return true
			}
			if button.LoginURL != nil && button.LoginURL.URL != "" {
				return true
			}
		}
	}
	return false
}

func inlineKeyboardRows(message *tgbotapi.Message) [][]tgbotapi.InlineKeyboardButton {
	if message == nil || message.ReplyMarkup == nil {
		return nil
	}
	return message.ReplyMarkup.InlineKeyboard
}

func messageIsForward(message *tgbotapi.Message) bool {
	return message.ForwardFrom != nil ||
		message.ForwardFromChat != nil ||
		message.ForwardFromMessageID != 0 ||
		message.ForwardSignature != "" ||
		message.ForwardSenderName != "" ||
		message.ForwardDate != 0 ||
		message.IsAutomaticForward
}

func messageHasMedia(message *tgbotapi.Message) bool {
	return message.Animation != nil ||
		message.Audio != nil ||
		message.Document != nil ||
		len(message.Photo) > 0 ||
		message.Sticker != nil ||
		message.Video != nil ||
		message.VideoNote != nil ||
		message.Voice != nil ||
		message.Contact != nil ||
		message.Game != nil ||
		message.Poll != nil ||
		message.Venue != nil ||
		message.Location != nil ||
		message.Dice != nil
}

func logSuspiciousMessage(cfg config.Config, message *tgbotapi.Message) {
	if !cfg.DebugLogSuspiciousMessages || message == nil {
		return
	}
	if !messageHasSuspiciousSurface(message) {
		return
	}

	raw, err := json.Marshal(message)
	if err != nil {
		log.Printf("debug suspicious message marshal failed: %v", err)
		return
	}
	log.Printf("debug suspicious message raw=%s", string(raw))
}

func messageHasSuspiciousSurface(message *tgbotapi.Message) bool {
	return messageHasLink(message) ||
		message.ReplyMarkup != nil ||
		messageIsForward(message) ||
		messageHasMedia(message)
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

func answerCallback(bot telegramClient, queryID, text string) {
	callback := tgbotapi.NewCallback(queryID, text)
	_, _ = bot.Request(callback)
}

func userMention(user *tgbotapi.User) string {
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		if user.UserName != "" {
			name = user.UserName
		} else {
			name = "Пользователь"
		}
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

func safeTelegramError(err error, token string) string {
	if err == nil {
		return ""
	}
	return redactToken(err.Error(), token)
}
