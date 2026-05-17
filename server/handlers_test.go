package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleIssue_Success(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	handler := handleIssue(store)

	req := httptest.NewRequest(http.MethodPost, "/nonce/issue", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp issueReponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Nonce == "" {
		t.Error("Response nonce is empty")
	}

	if resp.TTL != "5m0s" {
		t.Errorf("Expected TTL '5m0s', got '%s'", resp.TTL)
	}

	// Verify the nonce is actually in the store and can be consumed
	if err := store.Consume(resp.Nonce); err != nil {
		t.Errorf("Issued nonce is not in store: %v", err)
	}
}

func TestHandleIssue_ResponseContentType(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	handler := chain(
		handleIssue(store),
		setJSON,
	)

	req := httptest.NewRequest(http.MethodPost, "/nonce/issue", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestHandleConsume_Valid(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonce, _ := store.Issue()

	handler := handleConsume(store)
	body := consumeRequest{Nonce: nonce}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp consumeResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Valid {
		t.Error("Expected Valid=true for valid nonce")
	}

	if resp.Message != "ok" {
		t.Errorf("Expected message 'ok', got '%s'", resp.Message)
	}
}

func TestHandleConsume_NotFound(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)

	handler := handleConsume(store)
	body := consumeRequest{Nonce: "nonexistent"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var resp consumeResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Valid {
		t.Error("Expected Valid=false for nonexistent nonce")
	}

	if resp.Message != "nonce not found" {
		t.Errorf("Expected message 'nonce not found', got '%s'", resp.Message)
	}
}

func TestHandleConsume_Expired(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)
	nonce, _ := store.Issue()
	time.Sleep(10 * time.Millisecond)

	handler := handleConsume(store)
	body := consumeRequest{Nonce: nonce}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("Expected status %d (Gone), got %d", http.StatusGone, w.Code)
	}

	var resp consumeResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Message != "nonce expired" {
		t.Errorf("Expected message 'nonce expired', got '%s'", resp.Message)
	}
}

func TestHandleConsume_AlreadyUsed(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonce, _ := store.Issue()
	store.Consume(nonce)

	handler := handleConsume(store)
	body := consumeRequest{Nonce: nonce}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status %d (Conflict), got %d", http.StatusConflict, w.Code)
	}

	var resp consumeResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Message != "nonce already used" {
		t.Errorf("Expected message 'nonce already used', got '%s'", resp.Message)
	}
}

func TestHandleConsume_InvalidJSON(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	handler := handleConsume(store)

	invalidJSON := "invalid json"
	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader([]byte(invalidJSON)))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if !bytes.Contains(body, []byte("invalid request body")) {
		t.Errorf("Expected 'invalid request body' in error, got: %s", string(body))
	}
}

func TestHandleConsume_EmptyNonce(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	handler := handleConsume(store)

	body := consumeRequest{Nonce: ""}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	respBody, _ := io.ReadAll(w.Body)
	if !bytes.Contains(respBody, []byte("nonce field is required")) {
		t.Errorf("Expected 'nonce field is required' in error, got: %s", string(respBody))
	}
}

func TestHandleConsume_ResponseContentType(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)
	nonce, _ := store.Issue()

	handler := chain(
		handleConsume(store),
		setJSON,
	)

	body := consumeRequest{Nonce: nonce}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/nonce/consume", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}
