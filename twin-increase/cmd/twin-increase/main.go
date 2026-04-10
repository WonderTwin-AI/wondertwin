// twin-increase is a WonderTwin twin that simulates the Increase banking infrastructure API.
// It captures banking operations including accounts, ACH transfers, wire transfers,
// and card issuance, providing admin endpoints for testing and simulation.
//
// SDK compatibility target: github.com/Increase/increase-go
// Integration method: Override base URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-increase/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-increase")
	if cfg.Port == 0 {
		cfg.Port = 4123
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// Webhook dispatcher for events
	dispatcher := webhook.NewDispatcher(webhook.Config{
		URL:         cfg.WebhookURL,
		EventPrefix: "evt",
		MaxRetries:  3,
		Logger:      twin.Logger,
	})

	// API handlers
	apiHandler := api.NewHandler(memStore, dispatcher, twin.Middleware())
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)

	// Load seed data if provided
	if cfg.SeedFile != "" {
		data, err := os.ReadFile(cfg.SeedFile)
		if err != nil {
			log.Fatalf("failed to read seed file: %v", err)
		}
		if err := memStore.LoadState(data); err != nil {
			log.Fatalf("failed to load seed data: %v", err)
		}
		twin.Logger.Info("loaded seed data", "file", cfg.SeedFile)
	}

	twin.Logger.Info("twin-increase ready",
		"port", cfg.Port,
		"phase", "1 (Core Banking - MVP)",
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
