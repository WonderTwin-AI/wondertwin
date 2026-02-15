// twin-posthog is a WonderTwin twin that simulates the PostHog event capture API.
// It captures analytics events and provides admin endpoints
// for inspecting captured events and managing feature flags.
//
// SDK compatibility target: PostHog JS/Go SDK
// Integration method: Override API host
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-posthog")
	if cfg.Port == 0 {
		cfg.Port = 12114
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware())
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

	twin.Logger.Info("twin-posthog ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
