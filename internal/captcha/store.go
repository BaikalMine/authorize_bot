package captcha

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type Challenge struct {
	ChatID    int64
	UserID    int64
	MessageID int
	Question  string
	Answer    int
	Options   []int
	ExpiresAt time.Time
}

type Store struct {
	mu         sync.RWMutex
	challenges map[string]Challenge
}

func NewStore() *Store {
	return &Store{
		challenges: make(map[string]Challenge),
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
	s.challenges[key(chatID, userID)] = challenge
	s.mu.Unlock()

	return challenge, nil
}

func (s *Store) SetMessageID(chatID, userID int64, messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	challenge, ok := s.challenges[key(chatID, userID)]
	if !ok {
		return
	}
	challenge.MessageID = messageID
	s.challenges[key(chatID, userID)] = challenge
}

func (s *Store) Get(chatID, userID int64) (Challenge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	challenge, ok := s.challenges[key(chatID, userID)]
	return challenge, ok
}

func (s *Store) Delete(chatID, userID int64) {
	s.mu.Lock()
	delete(s.challenges, key(chatID, userID))
	s.mu.Unlock()
}

func (s *Store) Expired(now time.Time) []Challenge {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []Challenge
	for key, challenge := range s.challenges {
		if now.After(challenge.ExpiresAt) {
			expired = append(expired, challenge)
			delete(s.challenges, key)
		}
	}
	return expired
}

func key(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
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
