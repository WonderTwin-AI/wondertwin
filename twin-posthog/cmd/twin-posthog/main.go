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
	pkgevents "github.com/wondertwin-ai/wondertwin/twinkit/events"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-posthog/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-posthog")
	if cfg.Port == 0 {
		cfg.Port = 4114
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// Telemetry emitter — always created, enabled via env var or admin config.
	telemetryEnabled := os.Getenv("WT_TELEMETRY_REQUIRED") == "true"
	emitter := telemetry.NewEmitter(telemetry.Config{
		Enabled:      telemetryEnabled,
		CollectorURL: os.Getenv("WT_TELEMETRY_COLLECTOR_URL"),
		IngestKey:    os.Getenv("WT_TELEMETRY_INGEST_KEY"),
		OrgID:        os.Getenv("WT_TELEMETRY_ORG_ID"),
		TwinName:     "posthog",
		TwinVersion:  "0.1.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Events engine for event ingestion, identity resolution + telemetry bridge.
	eventsEngine := pkgevents.NewEngine(
		pkgevents.WithClock(memStore.Clock),
		pkgevents.WithTelemetry(emitter),
	)

	// Quirks engine — loaded at runtime from Content API intelligence.
	quirksEngine := quirks.NewEngine()

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware(), emitter, quirksEngine, eventsEngine)
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.SetQuirkStore(quirksEngine.AdminAdapter())
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
