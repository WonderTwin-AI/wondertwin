// twin-TEMPLATE is a WonderTwin twin that simulates the TEMPLATE service API.
// Replace TEMPLATE throughout with your service name (e.g., "acme").
//
// SDK compatibility target: github.com/your-org/your-sdk
// Integration method: Override base URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-TEMPLATE/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-TEMPLATE")
	if cfg.Port == 0 {
		cfg.Port = 4200 // Choose a unique port for your twin
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

	twin.Logger.Info("twin-TEMPLATE ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
