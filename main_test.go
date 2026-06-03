package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"autorize-bot-tg/internal/captcha"
	"autorize-bot-tg/internal/config"
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

func TestHandleMessageSkipsCaptchaWhenRestrictFails(t *testing.T) {
	bot := &fakeTelegramClient{requestErr: errors.New("restrict failed")}
	store := captcha.NewStore(captcha.Limits{})
	cfg := config.Config{BotToken: "token", CaptchaTimeout: time.Minute}
	message := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -100},
		NewChatMembers: []tgbotapi.User{
			{ID: 42, FirstName: "New"},
		},
	}

	handleMessage(bot, 1, store, cfg, message)

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
	cfg := config.Config{BotToken: "token", CaptchaTimeout: time.Minute}
	message := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -100},
		NewChatMembers: []tgbotapi.User{
			{ID: 42, FirstName: "New"},
		},
	}

	handleMessage(bot, 1, store, cfg, message)

	if bot.sendCalls != 1 {
		t.Fatalf("expected one captcha send attempt, got %d", bot.sendCalls)
	}
	if _, ok := store.Get(-100, 42); ok {
		t.Fatal("expected challenge to be deleted after captcha send failure")
	}
}

type fakeTelegramClient struct {
	requestErr error
	sendErr    error
	sendCalls  int
}

func (f *fakeTelegramClient) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return nil, f.requestErr
}

func (f *fakeTelegramClient) Send(tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.sendCalls++
	return tgbotapi.Message{MessageID: 123}, f.sendErr
}
