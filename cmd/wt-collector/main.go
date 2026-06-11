// wt-collector is the WonderTwin telemetry collector service.
// It receives anonymized behavioral observations from twin binaries
// and stores them for the intelligence pipeline.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/api"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/store"
)

// minIngestKeyLen is the boot-time floor for WT_COLLECTOR_INGEST_KEY.
// Mirrors the wt-auth WT_AUTH_SIGNING_KEY floor from the pre-deletion
// hardening (commit 5e48a7a). 32 bytes ≈ 256 bits of entropy when the
// operator follows the deploy doc (random-from-/dev/urandom); anything
// shorter is either a typo or a left-over dev value, both of which we
// want to refuse loudly rather than silently accept.
const minIngestKeyLen = 32

// validateIngestKey returns nil iff key is at least minIngestKeyLen
// bytes. Extracted so the test suite can cover the floor without
// spawning the binary.
func validateIngestKey(key string) error {
	if len(key) < minIngestKeyLen {
		return fmt.Errorf("WT_COLLECTOR_INGEST_KEY must be at least %d bytes, got %d", minIngestKeyLen, len(key))
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	port := flag.Int("port", 4400, "HTTP listen port")
	dbPath := flag.String("db", "", "Path to LibSQL database file")
	flag.Parse()

	ingestKey := os.Getenv("WT_COLLECTOR_INGEST_KEY")
	if err := validateIngestKey(ingestKey); err != nil {
		slog.Error("ingest key rejected", "err", err.Error())
		os.Exit(1)
	}

	if *dbPath == "" {
		if v := os.Getenv("WT_COLLECTOR_DB_PATH"); v != "" {
			*dbPath = v
		} else {
			*dbPath = "wt-collector.db"
		}
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		slog.Error("opening database", "err", err.Error(), "path", *dbPath)
		os.Exit(1)
	}
	defer db.Close()

	events := store.NewEventStore(db)
	handler := api.NewHandler(events, ingestKey)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	handler.Routes(r)

	if p := os.Getenv("WT_COLLECTOR_PORT"); p != "" {
		fmt.Sscanf(p, "%d", port)
	}

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "err", err.Error())
		}
	}()

	slog.Info("wt-collector starting", "addr", addr, "db", *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err.Error())
		os.Exit(1)
	}
	slog.Info("wt-collector stopped cleanly")
}
