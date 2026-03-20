// twin-loyaltylion is a WonderTwin twin that simulates the LoyaltyLion REST API v2.
// It implements customer management, points operations, reward redemption, and activity recording.
//
// SDK compatibility target: LoyaltyLion REST API v2
// Integration method: override base URL in HTTP client, Basic auth (token:secret)
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/workspace"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-loyaltylion/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-loyaltylion")
	if cfg.Port == 0 {
		cfg.Port = 8090
	}

	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.SeedDefaults()

	// Telemetry emitter — always created, enabled via env var or admin config.
	telemetryEnabled := os.Getenv("WT_TELEMETRY_REQUIRED") == "true"
	emitter := telemetry.NewEmitter(telemetry.Config{
		Enabled:      telemetryEnabled,
		CollectorURL: os.Getenv("WT_TELEMETRY_COLLECTOR_URL"),
		IngestKey:    os.Getenv("WT_TELEMETRY_INGEST_KEY"),
		OrgID:        os.Getenv("WT_TELEMETRY_ORG_ID"),
		TwinName:     "loyaltylion",
		TwinVersion:  "0.1.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Workspace engine for customer/reward entity lifecycle + telemetry bridge.
	wsEngine := workspace.NewEngine(
		workspace.WithClock(memStore.Clock),
		workspace.WithTelemetry(emitter),
		workspace.WithWorkflow(workspace.WorkflowConfig{
			EntityType:     "claimed_reward",
			InitialStatus:  "claimed",
			TerminalStates: []string{"refunded"},
			Transitions: map[string][]string{
				"claimed":  {"refunded"},
				"refunded": {},
			},
		}),
	)

	// Quirks engine — loaded at runtime from Content API intelligence.
	quirksEngine := quirks.NewEngine()

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware(), emitter, quirksEngine, wsEngine)
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.SetQuirkStore(quirksEngine.AdminAdapter())
	adminHandler.Routes(twin.Router)

	// Load seed data if provided (overrides defaults)
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

	twin.Logger.Info("twin-loyaltylion ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
