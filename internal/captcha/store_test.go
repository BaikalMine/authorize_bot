package captcha

import (
	"testing"
	"time"
)

func TestCreateChallenge(t *testing.T) {
	store := NewStore(Limits{})

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
	store := NewStore(Limits{})
	challenge, err := store.Create(-100, 42, time.Nanosecond)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	expired := store.Expired(time.Now().Add(time.Second), 0)
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

func TestGetValidRemovesExpiredChallenge(t *testing.T) {
	store := NewStore(Limits{})
	challenge, err := store.Create(-100, 42, -time.Second)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	expiredChallenge, ok, expired := store.GetValid(challenge.ChatID, challenge.UserID, time.Now())
	if ok {
		t.Fatal("expected expired challenge to be rejected")
	}
	if !expired {
		t.Fatal("expected expired flag")
	}
	if expiredChallenge.UserID != challenge.UserID {
		t.Fatalf("unexpected expired challenge user: %d", expiredChallenge.UserID)
	}
	if _, ok := store.Get(challenge.ChatID, challenge.UserID); ok {
		t.Fatal("expected expired challenge to be removed")
	}
}

func TestRecordFailedAttemptDeletesAfterLimit(t *testing.T) {
	store := NewStore(Limits{})
	challenge, err := store.Create(-100, 42, time.Minute)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	remaining, locked := store.RecordFailedAttempt(challenge.ChatID, challenge.UserID, 2)
	if locked {
		t.Fatal("expected first failed attempt to keep challenge active")
	}
	if remaining != 1 {
		t.Fatalf("expected 1 remaining attempt, got %d", remaining)
	}

	remaining, locked = store.RecordFailedAttempt(challenge.ChatID, challenge.UserID, 2)
	if !locked {
		t.Fatal("expected second failed attempt to lock challenge")
	}
	if remaining != 0 {
		t.Fatalf("expected no remaining attempts, got %d", remaining)
	}
	if _, ok := store.Get(challenge.ChatID, challenge.UserID); ok {
		t.Fatal("expected locked challenge to be removed")
	}
}

func TestCreateRejectsWhenGlobalLimitReached(t *testing.T) {
	store := NewStore(Limits{MaxActive: 1})
	if _, err := store.Create(-100, 1, time.Minute); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if _, err := store.Create(-100, 2, time.Minute); err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached, got %v", err)
	}
}

func TestCreateRejectsWhenChatLimitReached(t *testing.T) {
	store := NewStore(Limits{MaxActivePerChat: 1})
	if _, err := store.Create(-100, 1, time.Minute); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if _, err := store.Create(-100, 2, time.Minute); err != ErrChatLimitReached {
		t.Fatalf("expected ErrChatLimitReached, got %v", err)
	}
	if _, err := store.Create(-200, 3, time.Minute); err != nil {
		t.Fatalf("expected another chat to be allowed, got %v", err)
	}
}

func TestExpiredHonorsLimit(t *testing.T) {
	store := NewStore(Limits{})
	for userID := int64(1); userID <= 3; userID++ {
		if _, err := store.Create(-100, userID, -time.Second); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	expired := store.Expired(time.Now(), 2)
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired challenges, got %d", len(expired))
	}

	remaining := 0
	for userID := int64(1); userID <= 3; userID++ {
		if _, ok := store.Get(-100, userID); ok {
			remaining++
		}
	}
	if remaining != 1 {
		t.Fatalf("expected 1 challenge to remain for next cleanup batch, got %d", remaining)
	}
}
