// twin-smile is a WonderTwin twin that simulates the Smile.io rewards platform API.
// It implements customer lookup, points redemption, and points refund.
//
// SDK compatibility target: Smile.io REST API v1
// Integration method: override base URL in HTTP client
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-smile/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-smile")
	if cfg.Port == 0 {
		cfg.Port = 8087
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

	twin.Logger.Info("twin-smile ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
