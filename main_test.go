package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"autorize-bot-tg/internal/captcha"
	"autorize-bot-tg/internal/config"
	"autorize-bot-tg/internal/probation"
)

func TestParseCallback(t *testing.T) {
	chatID, userID, answer, ok := parseCallback("captcha:-100:42:17")
	if !ok {
		t.Fatal("expected callback to parse")
	}
	if chatID != -100 || userID != 42 || answer != 17 {
		t.Fatalf("unexpected parsed values: chat=%d user=%d answer=%d", chatID, userID, answer)
	}
}

func TestParseCallbackRejectsInvalidData(t *testing.T) {
	if _, _, _, ok := parseCallback("other:-100:42:17"); ok {
		t.Fatal("expected invalid prefix to be rejected")
	}
	if _, _, _, ok := parseCallback("captcha:-100:bad:17"); ok {
		t.Fatal("expected invalid user id to be rejected")
	}
}

func TestSafeTelegramErrorRedactsToken(t *testing.T) {
	token := "123456:SECRET"
	err := errors.New(`Post "https://api.telegram.org/bot123456:SECRET/sendMessage": timeout`)

	got := safeTelegramError(err, token)
	if strings.Contains(got, token) {
		t.Fatalf("expected token to be redacted, got %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("expected redacted marker, got %q", got)
	}
}

func TestUserMentionPrefersDisplayNameOverUsername(t *testing.T) {
	user := &tgbotapi.User{
		ID:        42,
		UserName:  "telegram_username",
		FirstName: "Display",
		LastName:  "Name",
	}

	got := userMention(user)
	if strings.Contains(got, "@telegram_username") {
		t.Fatalf("expected display name instead of username, got %q", got)
	}
	if !strings.Contains(got, "Display Name") {
		t.Fatalf("expected display name, got %q", got)
	}
}

func TestHandleMessageSkipsCaptchaWhenRestrictFails(t *testing.T) {
	bot := &fakeTelegramClient{requestErr: errors.New("restrict failed")}
	store := captcha.NewStore(captcha.Limits{})
	probationStore := probation.NewStore()
	cfg := config.Config{BotToken: "token", CaptchaTimeout: time.Minute}
	message := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -100},
		NewChatMembers: []tgbotapi.User{
			{ID: 42, FirstName: "New"},
		},
	}

	handleMessage(bot, 1, store, probationStore, cfg, message)

	if bot.sendCalls != 0 {
		t.Fatalf("expected captcha not to be sent after restrict failure, got %d sends", bot.sendCalls)
	}
	if _, ok := store.Get(-100, 42); ok {
		t.Fatal("expected no challenge after restrict failure")
	}
}

func TestHandleMessageDeletesChallengeWhenCaptchaSendFails(t *testing.T) {
	bot := &fakeTelegramClient{sendErr: errors.New("send failed")}
	store := captcha.NewStore(captcha.Limits{})
	probationStore := probation.NewStore()
	cfg := config.Config{BotToken: "token", CaptchaTimeout: time.Minute}
	message := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -100},
		NewChatMembers: []tgbotapi.User{
			{ID: 42, FirstName: "New"},
		},
	}

	handleMessage(bot, 1, store, probationStore, cfg, message)

	if bot.sendCalls != 1 {
		t.Fatalf("expected one captcha send attempt, got %d", bot.sendCalls)
	}
	if _, ok := store.Get(-100, 42); ok {
		t.Fatal("expected challenge to be deleted after captcha send failure")
	}
}

func TestIsProbationSpamDetectsLinks(t *testing.T) {
	cfg := config.Config{ProbationBlockLinks: true}
	message := &tgbotapi.Message{Text: "join https://example.com now"}

	if !isProbationSpam(message, cfg) {
		t.Fatal("expected link message to be treated as probation spam")
	}
}

func TestIsProbationSpamAllowsPlainText(t *testing.T) {
	cfg := config.Config{
		ProbationBlockLinks:    true,
		ProbationBlockForwards: true,
		ProbationBlockMedia:    true,
	}
	message := &tgbotapi.Message{Text: "hello everyone"}

	if isProbationSpam(message, cfg) {
		t.Fatal("expected plain text message to be allowed")
	}
}

func TestIsKnownSpamMessageDetectsRemoteJobSpamWithTextLink(t *testing.T) {
	message := &tgbotapi.Message{
		Text: "УДАЛЁНКА с 📲\nот 18 лет\nВ неделю от 𝟒𝟎𝟎𝟎𝟎 ₽\nПишите ⤵️\nАнна",
		Entities: []tgbotapi.MessageEntity{
			{Type: "text_link", URL: "https://t.me/m/HIIm_goyNjM6"},
		},
	}

	if !isKnownSpamMessage(message) {
		t.Fatal("expected remote job spam to be detected")
	}
}

func TestIsKnownSpamMessageAllowsPlainLink(t *testing.T) {
	message := &tgbotapi.Message{Text: "docs https://example.com"}

	if isKnownSpamMessage(message) {
		t.Fatal("expected plain link without spam markers to be allowed")
	}
}

func TestHandleGlobalSpamMessageDeletesAndKicks(t *testing.T) {
	bot := &fakeTelegramClient{}
	cfg := config.Config{
		BotToken:         "token",
		SpamGuardEnabled: true,
		SpamGuardKick:    true,
	}
	message := &tgbotapi.Message{
		MessageID: 11,
		Chat:      &tgbotapi.Chat{ID: -100},
		From:      &tgbotapi.User{ID: 42, FirstName: "Spam"},
		Text:      "УДАЛЁНКА от 18 лет, пишите https://t.me/m/HIIm_goyNjM6",
	}

	if !handleGlobalSpamMessage(bot, cfg, message) {
		t.Fatal("expected global spam handler to act")
	}
	if bot.requestCalls < 3 {
		t.Fatalf("expected delete, ban, and unban requests, got %d", bot.requestCalls)
	}
}

func TestHandleProbationMessageDeletesAndKicksSpam(t *testing.T) {
	bot := &fakeTelegramClient{}
	probationStore := probation.NewStore()
	cfg := config.Config{
		BotToken:               "token",
		KickOnTimeout:          true,
		ProbationEnabled:       true,
		ProbationBlockLinks:    true,
		ProbationBlockMedia:    true,
		ProbationBlockForwards: true,
	}
	probationStore.Add(-100, 42, time.Minute, time.Now())
	message := &tgbotapi.Message{
		MessageID: 10,
		Chat:      &tgbotapi.Chat{ID: -100},
		From:      &tgbotapi.User{ID: 42, FirstName: "New"},
		Text:      "spam https://example.com",
	}

	handleProbationMessage(bot, probationStore, cfg, message)

	if bot.requestCalls < 3 {
		t.Fatalf("expected delete, ban, and unban requests, got %d", bot.requestCalls)
	}
	if probationStore.Active(-100, 42, time.Now()) {
		t.Fatal("expected probation entry to be removed")
	}
}

type fakeTelegramClient struct {
	requestErr   error
	sendErr      error
	requestCalls int
	sendCalls    int
}

func (f *fakeTelegramClient) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.requestCalls++
	return nil, f.requestErr
}

func (f *fakeTelegramClient) Send(tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.sendCalls++
	return tgbotapi.Message{MessageID: 123}, f.sendErr
}
