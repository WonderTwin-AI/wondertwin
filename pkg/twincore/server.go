// Package twincore provides the base HTTP server, CLI flags, middleware chain,
// and response helpers shared by all WonderTwin twins.
package twincore

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// Config holds the common configuration for all twins, parsed from CLI flags.
type Config struct {
	Port       int
	Latency    time.Duration
	FailRate   float64
	WebhookURL string
	SeedFile   string
	Verbose    bool
	Name       string // twin name for logging
}

// ParseFlags parses common CLI flags and returns a Config.
// The twinName is used for logging and identification.
func ParseFlags(twinName string) *Config {
	cfg := &Config{Name: twinName}
	flag.IntVar(&cfg.Port, "port", 0, "HTTP listen port (default: auto-assigned)")
	flag.DurationVar(&cfg.Latency, "latency", 0, "Base simulated latency")
	flag.Float64Var(&cfg.FailRate, "fail-rate", 0.0, "Random failure rate 0.0-1.0")
	flag.StringVar(&cfg.WebhookURL, "webhook-url", "", "URL to send webhooks to")
	flag.StringVar(&cfg.SeedFile, "seed-file", "", "Path to JSON fixture for initial state")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable request/response logging")
	flag.Parse()

	if cfg.Port == 0 {
		if p := os.Getenv("PORT"); p != "" {
			fmt.Sscanf(p, "%d", &cfg.Port)
		}
		if cfg.Port == 0 {
			cfg.Port = 8080
		}
	}

	return cfg
}

// Twin is the base server for a WonderTwin twin. It wraps a chi router with
// common middleware and provides lifecycle management.
type Twin struct {
	Config *Config
	Router *chi.Mux
	Logger *slog.Logger
	mw     *Middleware
}

// New creates a new Twin with the given config.
func New(cfg *Config) *Twin {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	if cfg.Verbose {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	r := chi.NewRouter()
	mw := NewMiddleware(cfg, logger)

	// Common middleware stack
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.CORS)
	r.Use(mw.RequestLog)
	if cfg.Latency > 0 {
		r.Use(mw.LatencyInjection)
	}
	if cfg.FailRate > 0 {
		r.Use(mw.RandomFailure)
	}

	return &Twin{
		Config: cfg,
		Router: r,
		Logger: logger,
		mw:     mw,
	}
}

// Middleware returns the middleware instance for external access (e.g., fault injection).
func (t *Twin) Middleware() *Middleware {
	return t.mw
}

// Serve starts the HTTP server and blocks until shutdown signal.
func (t *Twin) Serve() error {
	addr := fmt.Sprintf(":%d", t.Config.Port)

	srv := &http.Server{
		Addr:         addr,
		Handler:      t.Router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		t.Logger.Info("starting twin", "name", t.Config.Name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-done
	t.Logger.Info("shutting down twin", "name", t.Config.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// ServeHTTP implements http.Handler so Twin can be used directly in tests.
func (t *Twin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.Router.ServeHTTP(w, r)
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

// Error writes a JSON error response.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    http.StatusText(status),
			"code":    status,
		},
	})
}

// StripeError writes an error response in Stripe's error format.
func StripeError(w http.ResponseWriter, status int, errType, code, message string) {
	JSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    errType,
			"code":    code,
			"message": message,
		},
	})
}
