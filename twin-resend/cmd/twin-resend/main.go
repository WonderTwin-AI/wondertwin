// twin-resend is a WonderTwin twin that simulates the Resend email API.
// It captures email send calls and provides admin endpoints
// for inspecting sent emails.
//
// SDK compatibility target: github.com/resend/resend-go/v2
// Integration method: Override base URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/messaging"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-resend")
	if cfg.Port == 0 {
		cfg.Port = 4113
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
		TwinName:     "resend",
		TwinVersion:  "0.1.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Messaging engine for email lifecycle + telemetry bridge.
	msgEngine := messaging.NewEngine(
		messaging.WithClock(memStore.Clock),
		messaging.WithTelemetry(emitter),
		messaging.WithLifecycle(messaging.ChannelEmail, &messaging.Lifecycle{
			InitialStatus: messaging.StatusQueued,
			Transitions: map[messaging.MessageStatus][]messaging.MessageStatus{
				messaging.StatusQueued:    {messaging.StatusSending, messaging.StatusDropped},
				messaging.StatusSending:   {messaging.StatusSent},
				messaging.StatusSent:      {messaging.StatusDelivered, messaging.StatusBounced},
				messaging.StatusDelivered: {},
				messaging.StatusBounced:   {},
				messaging.StatusDropped:   {},
			},
			AutoAdvance: map[messaging.MessageStatus]messaging.AutoAdvanceConfig{
				messaging.StatusQueued:  {To: messaging.StatusSending},
				messaging.StatusSending: {To: messaging.StatusSent},
				messaging.StatusSent:    {To: messaging.StatusDelivered},
			},
		}),
	)

	// Quirks engine — loaded at runtime from Content API intelligence.
	quirksEngine := quirks.NewEngine()

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware(), emitter, quirksEngine, msgEngine)
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

	twin.Logger.Info("twin-resend ready",
		"port", cfg.Port,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
