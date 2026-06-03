package captcha

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var (
	// ErrLimitReached is returned when the global active challenge limit is full.
	ErrLimitReached = errors.New("captcha challenge limit reached")

	// ErrChatLimitReached is returned when a chat-specific challenge limit is full.
	ErrChatLimitReached = errors.New("captcha chat challenge limit reached")
)

// Limits controls how many active challenges the in-memory store accepts.
type Limits struct {
	MaxActive        int
	MaxActivePerChat int
}

type Challenge struct {
	ChatID    int64
	UserID    int64
	MessageID int
	Question  string
	Answer    int
	Options   []int
	Attempts  int
	ExpiresAt time.Time
}

type Store struct {
	mu         sync.RWMutex
	limits     Limits
	challenges map[challengeKey]Challenge
	chatCounts map[int64]int
}

type challengeKey struct {
	chatID int64
	userID int64
}

func NewStore(limits Limits) *Store {
	return &Store{
		limits:     limits,
		challenges: make(map[challengeKey]Challenge),
		chatCounts: make(map[int64]int),
	}
}

func (s *Store) Create(chatID, userID int64, timeout time.Duration) (Challenge, error) {
	left, err := randomInt(3, 18)
	if err != nil {
		return Challenge{}, err
	}
	right, err := randomInt(2, 12)
	if err != nil {
		return Challenge{}, err
	}

	answer := left + right
	options := []int{answer}
	for len(options) < 4 {
		delta, err := randomInt(-7, 7)
		if err != nil {
			return Challenge{}, err
		}
		value := answer + delta
		if value <= 0 || contains(options, value) {
			continue
		}
		options = append(options, value)
	}

	if err := shuffle(options); err != nil {
		return Challenge{}, err
	}

	challenge := Challenge{
		ChatID:    chatID,
		UserID:    userID,
		Question:  fmt.Sprintf("%d + %d = ?", left, right),
		Answer:    answer,
		Options:   options,
		ExpiresAt: time.Now().Add(timeout),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(chatID, userID)
	if _, exists := s.challenges[k]; !exists {
		if s.limits.MaxActive > 0 && len(s.challenges) >= s.limits.MaxActive {
			return Challenge{}, ErrLimitReached
		}
		if s.limits.MaxActivePerChat > 0 && s.chatCounts[chatID] >= s.limits.MaxActivePerChat {
			return Challenge{}, ErrChatLimitReached
		}
		s.chatCounts[chatID]++
	}
	s.challenges[k] = challenge

	return challenge, nil
}

func (s *Store) SetMessageID(chatID, userID int64, messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(chatID, userID)
	challenge, ok := s.challenges[k]
	if !ok {
		return
	}
	challenge.MessageID = messageID
	s.challenges[k] = challenge
}

func (s *Store) Get(chatID, userID int64) (Challenge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	challenge, ok := s.challenges[key(chatID, userID)]
	return challenge, ok
}

func (s *Store) GetValid(chatID, userID int64, now time.Time) (challenge Challenge, ok bool, expired bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(chatID, userID)
	challenge, ok = s.challenges[k]
	if !ok {
		return Challenge{}, false, false
	}
	if !now.Before(challenge.ExpiresAt) {
		s.deleteLocked(k)
		return challenge, false, true
	}
	return challenge, true, false
}

func (s *Store) RecordFailedAttempt(chatID, userID int64, maxAttempts int) (remaining int, locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(chatID, userID)
	challenge, ok := s.challenges[k]
	if !ok {
		return 0, true
	}

	challenge.Attempts++
	if maxAttempts > 0 && challenge.Attempts >= maxAttempts {
		s.deleteLocked(k)
		return 0, true
	}
	s.challenges[k] = challenge

	if maxAttempts <= 0 {
		return 0, false
	}
	return maxAttempts - challenge.Attempts, false
}

func (s *Store) Delete(chatID, userID int64) {
	s.mu.Lock()
	s.deleteLocked(key(chatID, userID))
	s.mu.Unlock()
}

func (s *Store) Expired(now time.Time, limit int) []Challenge {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []Challenge
	for key, challenge := range s.challenges {
		if !now.Before(challenge.ExpiresAt) {
			expired = append(expired, challenge)
			s.deleteLocked(key)
			if limit > 0 && len(expired) >= limit {
				break
			}
		}
	}
	return expired
}

func (s *Store) deleteLocked(k challengeKey) {
	challenge, ok := s.challenges[k]
	if !ok {
		return
	}
	delete(s.challenges, k)
	s.chatCounts[challenge.ChatID]--
	if s.chatCounts[challenge.ChatID] <= 0 {
		delete(s.chatCounts, challenge.ChatID)
	}
}

func key(chatID, userID int64) challengeKey {
	return challengeKey{chatID: chatID, userID: userID}
}

func randomInt(min, max int) (int, error) {
	if max < min {
		return 0, fmt.Errorf("invalid random range %d..%d", min, max)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return min + int(n.Int64()), nil
}

func shuffle(values []int) error {
	for i := len(values) - 1; i > 0; i-- {
		j, err := randomInt(0, i)
		if err != nil {
			return err
		}
		values[i], values[j] = values[j], values[i]
	}
	return nil
}

func contains(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
