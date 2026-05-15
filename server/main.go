package main 

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	store := NewNonceStore(5 * time.Minute)

	// Run Purge on a ticker - store doesn't manage its own goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			store.Purge()
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/nonce/issue", chain(
		handleIssue(store),
		setJSON,
		requireMethod(http.MethodPost),
	))

	mux.HandleFunc("/nonce/consume", chain(
		handleConsume(store),
		setJSON,
		requireMethod(http.MethodPost),
	))

	srv := &http.Server{
		Addr: ":8080",
		Handler: mux,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	// Start server in a goroutine so we can listen for shutdown signals
	go func() {
		fmt.Println("Server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Block until SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down...")
	
	// Give in-flight requests up to 10 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "forced shutdown: %v\n", err)
	}

	fmt.Println("Server stopped")
}