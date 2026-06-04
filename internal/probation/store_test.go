package probation

import (
	"testing"
	"time"
)

func TestStoreActiveExpiresEntries(t *testing.T) {
	store := NewStore()
	now := time.Now()

	store.Add(-100, 42, time.Minute, now)
	if !store.Active(-100, 42, now.Add(30*time.Second)) {
		t.Fatal("expected probation entry to be active")
	}
	if store.Active(-100, 42, now.Add(2*time.Minute)) {
		t.Fatal("expected probation entry to expire")
	}
}

func TestExpiredHonorsLimit(t *testing.T) {
	store := NewStore()
	now := time.Now()
	for userID := int64(1); userID <= 3; userID++ {
		store.Add(-100, userID, -time.Second, now)
	}

	expired := store.Expired(now, 2)
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired entries, got %d", len(expired))
	}
}
