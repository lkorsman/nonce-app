package nonce

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Nonce represents a single-use token.
// Once Used is true, it can never be verified again.
type Nonce struct {
	Value     string
	ExpiresAt time.Time
	Used      bool
}

// NonceStore holds all issued nonces in memory.
type NonceStore struct {
	mu     sync.Mutex // Used to prevent race conditions
	nonces map[string]*Nonce
}

var (
	ErrNonceNotFound = errors.New("nonce not found")
	ErrNonceExpired  = errors.New("nonce has expired")
	ErrNonceUsed     = errors.New("nonce already used")
)

func NewNonceStore() *NonceStore {
	return &NonceStore{
		nonces: make(map[string]*Nonce),
	}
}

func (ns *NonceStore) Generate(ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Encode to a hex string so it's safe to send in HTTP headers/JSON.
	// 32 bytes = 64 hex characters = 256 bits of entropy.
	value := hex.EncodeToString(b)

	ns.mu.Lock()
	defer ns.mu.Unlock()

	ns.nonces[value] = &Nonce{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
		Used:      false,
	}

	return value, nil
}

// Verify checks if a nonce is valid, unexpired, and unused.
// If all checks pass it marks the nonce as used atomically.
func (ns *NonceStore) Verify(value string) error {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	nonce, exists := ns.nonces[value]
	if !exists {
		return ErrNonceNotFound
	}

	if time.Now().After(nonce.ExpiresAt) {
		return ErrNonceExpired
	}

	if nonce.Used {
		return ErrNonceUsed
	}

	nonce.Used = true
	return nil
}

// StartCleanup launches a background goroutine that periodically
// removes expired nonces so memory doesn't grow forever.
func (ns *NonceStore) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ns.cleanup()
		}
	}()
}

func (ns *NonceStore) cleanup() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	now := time.Now()
	for key, nonce := range ns.nonces {
		if now.After(nonce.ExpiresAt) {
			delete(ns.nonces, key)
		}
	}
}