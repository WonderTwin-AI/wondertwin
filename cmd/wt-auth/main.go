// wt-auth is the WonderTwin authorization service. It validates org
// credentials, issues JIT tokens for twin binaries, and gates access
// to the Content API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/api"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/token"
)

// minSecretBytes is the lower bound on WT_AUTH_SIGNING_KEY and
// WT_AUTH_INTERNAL_KEY. The 2026-06-04 customer-launch audit flagged
// the previous "any length accepted" behavior. 32 bytes matches the
// platform-side secret floor (WT_API_KEY_HMAC_SECRET, etc.) and
// gives HMAC-SHA256 a key as wide as its output.
const minSecretBytes = 32

// Timeout values for *http.Server. ReadHeaderTimeout is the
// canonical Slowloris bound; the others are conservative defaults.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

// shutdownTimeout bounds how long srv.Shutdown will wait for in-
// flight requests to drain on SIGTERM. Past this we fall through
// and the process exits — any open connections get an abrupt RST.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("wt-auth exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Structured JSON logging from the start so a misconfig error
	// lands as parseable JSON, not free-text log.Printf.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	port := flag.Int("port", 4300, "HTTP listen port")
	dbPath := flag.String("db", "", "Path to LibSQL database file")
	flag.Parse()

	// Required env vars — fail fast with length floors.
	signingKey := os.Getenv("WT_AUTH_SIGNING_KEY")
	if signingKey == "" {
		return fmt.Errorf("WT_AUTH_SIGNING_KEY is required")
	}
	if len(signingKey) < minSecretBytes {
		return fmt.Errorf("WT_AUTH_SIGNING_KEY must be at least %d bytes (got %d)",
			minSecretBytes, len(signingKey))
	}
	internalKey := os.Getenv("WT_AUTH_INTERNAL_KEY")
	if internalKey == "" {
		return fmt.Errorf("WT_AUTH_INTERNAL_KEY is required")
	}
	if len(internalKey) < minSecretBytes {
		return fmt.Errorf("WT_AUTH_INTERNAL_KEY must be at least %d bytes (got %d)",
			minSecretBytes, len(internalKey))
	}

	// Database.
	if *dbPath == "" {
		if v := os.Getenv("WT_AUTH_DB_PATH"); v != "" {
			*dbPath = v
		} else {
			*dbPath = "wt-auth.db"
		}
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	licenses := store.NewLicenseStore(db)
	tokens := store.NewTokenStore(db)
	signer := token.NewSigner(signingKey)

	handler := api.NewHandler(licenses, tokens, signer, internalKey)

	r := chi.NewRouter()
	handler.Routes(r)

	// Override port from env.
	if p := os.Getenv("WT_AUTH_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			*port = v
		}
	}

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
		// ReadHeaderTimeout caps the slow-headers Slowloris attack.
		// ReadTimeout covers the full body read; both apply at the
		// connection level even before our handlers see the request.
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Graceful shutdown on SIGINT/SIGTERM. signal.NotifyContext returns
	// a ctx that gets cancelled when the signal fires; srv.Shutdown
	// then drains in-flight requests up to shutdownTimeout before
	// returning.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("wt-auth listening", "addr", addr, "db", *dbPath)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("wt-auth: shutdown signal received; draining")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("wt-auth: shutdown failed; some in-flight requests may have been dropped",
			"error", err)
		return err
	}
	slog.Info("wt-auth: clean exit")
	return nil
}
