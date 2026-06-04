package probation

import (
	"sync"
	"time"
)

type Entry struct {
	ChatID    int64
	UserID    int64
	ExpiresAt time.Time
}

type Store struct {
	mu      sync.RWMutex
	entries map[key]Entry
}

type key struct {
	chatID int64
	userID int64
}

func NewStore() *Store {
	return &Store{
		entries: make(map[key]Entry),
	}
}

func (s *Store) Add(chatID, userID int64, duration time.Duration, now time.Time) Entry {
	entry := Entry{
		ChatID:    chatID,
		UserID:    userID,
		ExpiresAt: now.Add(duration),
	}

	s.mu.Lock()
	s.entries[key{chatID: chatID, userID: userID}] = entry
	s.mu.Unlock()

	return entry
}

func (s *Store) Active(chatID, userID int64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key{chatID: chatID, userID: userID}
	entry, ok := s.entries[k]
	if !ok {
		return false
	}
	if !now.Before(entry.ExpiresAt) {
		delete(s.entries, k)
		return false
	}
	return true
}

func (s *Store) Delete(chatID, userID int64) {
	s.mu.Lock()
	delete(s.entries, key{chatID: chatID, userID: userID})
	s.mu.Unlock()
}

func (s *Store) Expired(now time.Time, limit int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []Entry
	for key, entry := range s.entries {
		if !now.Before(entry.ExpiresAt) {
			expired = append(expired, entry)
			delete(s.entries, key)
			if limit > 0 && len(expired) >= limit {
				break
			}
		}
	}
	return expired
}
