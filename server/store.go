package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrNonceNotFound = errors.New("nonce not found")
	ErrNonceExpired  = errors.New("nonce has expired")
	ErrNonceUsed     = errors.New("nonce already used")
)

// Nonce represents a single-use token.
// Once Used is true, it can never be verified again.
type nonce struct {
	value     string
	expiresAt time.Time
	usedAt 	  *time.Time // nil if unused - records WHEN it was used
}

// NonceStore holds all issued nonces in memory.
type NonceStore struct {
	mu     sync.RWMutex // Used to prevent race conditions
	nonces map[string]*nonce
	ttl time.Duration
}

func NewNonceStore(ttl time.Duration) *NonceStore {
	return &NonceStore{
		nonces: make(map[string]*nonce),
		ttl: ttl,
	}
}

// Issue generates and stores a new nonce, returning its value
func (s *NonceStore) Issue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	val := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nonces[val] = &nonce{
		value: val,
		expiresAt: time.Now().Add(s.ttl),
	}

	return val, nil
}

// Consume verifies a nonce is valid and marks it used in one atomic operation.
// After a successful Consume the nonce cannot be used again.
func (s *NonceStore) Consume(val string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.nonces[val]
	if !ok {
		return ErrNonceNotFound
	}
	if time.Now().After(n.expiresAt) {
		return ErrNonceExpired
	}
	if n.usedAt != nil {
		return ErrNonceUsed
	}

	now := time.Now()
	n.usedAt = &now
	return nil
}

// Purge removes expired nonces from the store.
// Call this on a ticker from main - it should not run itself.
func (s *NonceStore) Purge() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, n := range s.nonces {
		// Keep recently used nonces briefly so replay attempts
		// get ErrNonceUsed rather than ErrNonceNotFound
		// Once expired AND used (or expired AND grace period passed), remove.
		if now.After(n.expiresAt.Add(1 * time.Minute)) {
			delete(s.nonces, k)
		}
	}
}