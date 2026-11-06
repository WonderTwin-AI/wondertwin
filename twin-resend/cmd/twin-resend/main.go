// twin-resend is a WonderTwin twin that simulates the Resend email API.
// It captures email send calls and provides admin endpoints
// for inspecting sent emails.
//
// SDK compatibility target: github.com/resend/resend-go/v4 (v4.7.0)
// Integration method: Override base URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func main() {
	cfg := twincore.ParseFlags("twin-resend")
	if cfg.Port == 0 {
		cfg.Port = 4113
	}

	twin := twincore.New(cfg)
	memStore := store.New()
	// The seed of sim.DefaultRand, held by the store so /admin/reset
	// restores it and ids repeat from a fresh start.
	memStore.Seed(42)

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware())
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(apiHandler.ConfigProvider(twin))
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

	twin.Logger.Info("twin-resend ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
