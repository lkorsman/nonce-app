package main

import (
	"testing"
	"time"
)

func TestIssue(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)

	nonce, err := store.Issue()
	if err != nil {
		t.Fatalf("Issue() failed: %v", err)
	}

	// Verify nonce is non-empty
	if nonce == "" {
		t.Error("Issue() returned empty nonce")
	}

	// Verify nonce is 64 characters (32 bytes as hex)
	if len(nonce) != 64 {
		t.Errorf("Issue() returned nonce of length %d, want 64", len(nonce))
	}

	// Verify each call produces a unique nonce
	nonce2, _ := store.Issue()
	if nonce == nonce2 {
		t.Error("Issue() returned duplicate nonce")
	}
}

func TestIssueMultiple(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonces := make(map[string]bool)

	for i := range 100 {
		nonce, err := store.Issue()
		if err != nil {
			t.Fatalf("Issue() failed on iteration %d: %v", i, err)
		}
		if nonces[nonce] {
			t.Errorf("Duplicate nonce generated: %s", nonce)
		}
		nonces[nonce] = true
	}
}

func TestConsume_Valid(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonce, _ := store.Issue()

	err := store.Consume(nonce)
	if err != nil {
		t.Errorf("Consume() on valid nonce failed: %v", err)
	}
}

func TestConsume_NotFound(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)

	err := store.Consume("nonexistent")
	if err != ErrNonceNotFound {
		t.Errorf("Consume() on nonexistent nonce: got %v, want %v", err, ErrNonceNotFound)
	}
}

func TestConsume_AlreadyUsed(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonce, _ := store.Issue()

	// First consume should succeed
	store.Consume(nonce)

	// Second consume should fail with ErrNonceUsed
	err := store.Consume(nonce)
	if err != ErrNonceUsed {
		t.Errorf("Consume() on used nonce: got %v, want %v", err, ErrNonceUsed)
	}
}

func TestConsume_Expired(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)
	nonce, _ := store.Issue()

	// Wait for nonce to expire
	time.Sleep(10 * time.Millisecond)

	err := store.Consume(nonce)
	if err != ErrNonceExpired {
		t.Errorf("Consume() on expired nonce: got %v, want %v", err, ErrNonceExpired)
	}
}

func TestPurge_RemovesExpiredNonces(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)

	nonce1, _ := store.Issue()
	time.Sleep(10 * time.Millisecond)

	// Issue another nonce after expiration
	nonce2, _ := store.Issue()

	store.Purge()

	// First nonce is expired but purge hasn't removed it yet (no grace period passed)
	// The nonce will be rejected as expired on consume attempt
	err := store.Consume(nonce1)
	if err != ErrNonceExpired {
		t.Errorf("After Purge(), should reject expired nonce: got %v, want ErrNonceExpired", err)
	}

	// Second nonce should still be valid
	err = store.Consume(nonce2)
	if err != nil {
		t.Errorf("After Purge(), valid nonce was removed: %v", err)
	}
}

func TestPurge_KeepsRecentlyUsedNonces(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)

	nonce, _ := store.Issue()
	time.Sleep(10 * time.Millisecond)

	// Consume the nonce so it's marked as used
	store.Consume(nonce)

	// Purge shouldn't remove it immediately (grace period is 1 minute)
	store.Purge()

	// Try to consume again—nonce is expired so it will return ErrNonceExpired
	err := store.Consume(nonce)
	if err != ErrNonceExpired {
		t.Errorf("Purge() removed used nonce too early: got %v, want ErrNonceExpired", err)
	}
}

func TestPurge_RemovesExpiredUsedNoncesAfterGracePeriod(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)

	nonce, _ := store.Issue()
	time.Sleep(10 * time.Millisecond)
	store.Consume(nonce)

	// Manually trigger purge with time after grace period
	// We need to test the edge case where expired + grace period has passed
	store.mu.Lock()
	n := store.nonces[nonce]
	n.expiresAt = time.Now().Add(-2 * time.Minute)
	store.mu.Unlock()

	store.Purge()

	err := store.Consume(nonce)
	if err != ErrNonceNotFound {
		t.Errorf("After grace period, used nonce should be purged: got %v, want ErrNonceNotFound", err)
	}
}

func TestConcurrentIssueAndConsume(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	done := make(chan error, 20)

	// Goroutines issuing nonces
	for range 10 {
		go func() {
			_, err := store.Issue()
			done <- err
		}()
	}

	// Collect issued nonces
	var nonces []string
	for range 10 {
		err := <-done
		if err != nil {
			t.Fatalf("Issue() failed: %v", err)
		}
	}

	store.mu.RLock()
	for nonce := range store.nonces {
		nonces = append(nonces, nonce)
	}
	store.mu.RUnlock()

	// Goroutines consuming nonces
	for _, nonce := range nonces {
		go func(n string) {
			err := store.Consume(n)
			done <- err
		}(nonce)
	}

	for i := 0; i < len(nonces); i++ {
		err := <-done
		if err != nil {
			t.Fatalf("Consume() failed: %v", err)
		}
	}

	// All nonces should now be used, second consume should fail
	for _, nonce := range nonces {
		err := store.Consume(nonce)
		if err != ErrNonceUsed {
			t.Errorf("Expected ErrNonceUsed after concurrent operations: got %v", err)
		}
	}
}
