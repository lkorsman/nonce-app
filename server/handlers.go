package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

// issueResponse is what the client receives after requesting a nonce
type issueReponse struct {
	Nonce string `json:"nonce"`
	TTL   string `json:"ttl"`
}

// consumeRequest is what the client sneds when verifying a nonce
type consumeRequest struct {
	Nonce string `json:"nonce"`
}

// consumeResponse is what the client receives after verification
type consumeResponse struct {
	Valid   bool    `json:"valid"`
	Message string  `json:"message"`
}

func handleIssue(s *NonceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		val, err := s.Issue()
		if err != nil {
			http.Error(w, `{"error":"failed to generate nonce"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(issueReponse{
			Nonce: val,
			TTL: s.ttl.String(),
		})
	}
}

func handleConsume(store *NonceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req consumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Nonce == "" {
			http.Error(w, `{"error":"nonce field is required"}`, http.StatusBadRequest)
			return
		}

		err := store.Consume(req.Nonce)

		resp := consumeResponse{Valid: err == nil}
		switch {
        case err == nil:
            resp.Message = "ok"
        case errors.Is(err, ErrNonceNotFound):
            resp.Message = "nonce not found"
            w.WriteHeader(http.StatusNotFound)
        case errors.Is(err, ErrNonceExpired):
            resp.Message = "nonce expired"
            w.WriteHeader(http.StatusGone)
        case errors.Is(err, ErrNonceUsed):
            resp.Message = "nonce already used"
            w.WriteHeader(http.StatusConflict)
        default:
            resp.Message = "internal error"
            w.WriteHeader(http.StatusInternalServerError)
        }

		json.NewEncoder(w).Encode(resp)
	}
}
