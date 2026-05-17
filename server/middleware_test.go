package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireMethod_AllowedMethod(t *testing.T) {
	handler := requireMethod(http.MethodPost)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "success" {
		t.Errorf("Expected handler to be called, got: %s", string(body))
	}
}

func TestRequireMethod_WrongMethod(t *testing.T) {
	handler := requireMethod(http.MethodPost)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestRequireJSON_ValidContentType(t *testing.T) {
	handler := requireJSON(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "success" {
		t.Errorf("Expected handler to be called, got: %s", string(body))
	}
}

func TestRequireJSON_MissingContentType(t *testing.T) {
	handler := requireJSON(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("Expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
}

func TestRequireJSON_WrongContentType(t *testing.T) {
	handler := requireJSON(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("Expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
}

func TestSetJSON(t *testing.T) {
	handler := setJSON(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestChain_SingleMiddleware(t *testing.T) {
	callOrder := []string{}

	middleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "middleware")
			next(w, r)
		}
	}

	handler := chain(
		func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "handler")
			w.WriteHeader(http.StatusOK)
		},
		middleware,
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if len(callOrder) != 2 || callOrder[0] != "middleware" || callOrder[1] != "handler" {
		t.Errorf("Expected middleware to run before handler, got: %v", callOrder)
	}
}

func TestChain_MultipleMiddleware(t *testing.T) {
	callOrder := []string{}

	middleware1 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "middleware1")
			next(w, r)
		}
	}

	middleware2 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "middleware2")
			next(w, r)
		}
	}

	handler := chain(
		func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "handler")
			w.WriteHeader(http.StatusOK)
		},
		middleware1,
		middleware2,
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// middleware1 (first in list) should execute first, then middleware2
	if len(callOrder) != 3 || callOrder[0] != "middleware1" || callOrder[1] != "middleware2" || callOrder[2] != "handler" {
		t.Errorf("Expected middleware to run in order (1,2,handler), got: %v", callOrder)
	}
}

func TestChain_MiddlewareCanRejectRequest(t *testing.T) {
	handlerCalled := false

	middleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			// Don't call next - reject the request
		}
	}

	handler := chain(
		func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		},
		middleware,
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if handlerCalled {
		t.Error("Handler should not be called if middleware rejects")
	}

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestChain_NoMiddleware(t *testing.T) {
	handler := chain(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "success" {
		t.Errorf("Expected 'success', got: %s", string(body))
	}
}
