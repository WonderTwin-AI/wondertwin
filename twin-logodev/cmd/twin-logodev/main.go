package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-logodev")
	if cfg.Port == 0 {
		cfg.Port = 4116
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
		TwinName:     "logodev",
		TwinVersion:  "0.1.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Quirks engine — loaded at runtime from Content API intelligence.
	quirksEngine := quirks.NewEngine()

	apiHandler := api.NewHandler(memStore, twin.Middleware(), emitter, quirksEngine)
	apiHandler.Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.SetQuirkStore(quirksEngine.AdminAdapter())
	adminHandler.Routes(twin.Router)

	if cfg.SeedFile != "" {
		data, err := os.ReadFile(cfg.SeedFile)
		if err != nil {
			log.Fatalf("failed to read seed file: %v", err)
		}
		if err := memStore.LoadState(data); err != nil {
			log.Fatalf("failed to load seed data: %v", err)
		}
	}

	twin.Logger.Info("twin-logodev ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
