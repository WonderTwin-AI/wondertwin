// twin-algolia is a WonderTwin twin that simulates the Algolia Search API.
// It handles search, indexing, index management, settings, synonyms, rules,
// and API key management, backed by the twinkit/search engine.
//
// SDK compatibility target: github.com/algolia/algoliasearch-client-go
// Integration method: Override hosts
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/quirks"
	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	"github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-algolia/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-algolia")
	if cfg.Port == 0 {
		cfg.Port = 4123
	}

	twin := twincore.New(cfg)

	telemetryEnabled := os.Getenv("WT_TELEMETRY_REQUIRED") == "true"
	emitter := telemetry.NewEmitter(telemetry.Config{
		Enabled:      telemetryEnabled,
		CollectorURL: os.Getenv("WT_TELEMETRY_COLLECTOR_URL"),
		IngestKey:    os.Getenv("WT_TELEMETRY_INGEST_KEY"),
		OrgID:        os.Getenv("WT_TELEMETRY_ORG_ID"),
		TwinName:     "algolia",
		TwinVersion:  "0.1.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Search engine with telemetry bridge
	engine := search.NewEngine(
		search.WithClock(state.NewClock()),
		search.WithTelemetry(emitter),
	)

	memStore := store.New(engine)
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
		twin.Logger.Info("loaded seed data", "file", cfg.SeedFile)
	}

	twin.Logger.Info("twin-algolia ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
