package captcha

import (
	"testing"
	"time"
)

func TestCreateChallenge(t *testing.T) {
	store := NewStore()

	challenge, err := store.Create(-100, 42, time.Minute)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if challenge.ChatID != -100 || challenge.UserID != 42 {
		t.Fatalf("unexpected challenge owner: chat=%d user=%d", challenge.ChatID, challenge.UserID)
	}
	if challenge.Question == "" {
		t.Fatal("question is empty")
	}
	if len(challenge.Options) != 4 {
		t.Fatalf("expected 4 options, got %d", len(challenge.Options))
	}

	foundAnswer := false
	for _, option := range challenge.Options {
		if option == challenge.Answer {
			foundAnswer = true
		}
	}
	if !foundAnswer {
		t.Fatal("options do not contain the answer")
	}
}

func TestExpiredRemovesChallenges(t *testing.T) {
	store := NewStore()
	challenge, err := store.Create(-100, 42, time.Nanosecond)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	expired := store.Expired(time.Now().Add(time.Second))
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired challenge, got %d", len(expired))
	}
	if expired[0].UserID != challenge.UserID {
		t.Fatalf("unexpected expired challenge user: %d", expired[0].UserID)
	}

	if _, ok := store.Get(challenge.ChatID, challenge.UserID); ok {
		t.Fatal("expired challenge was not removed")
	}
}
