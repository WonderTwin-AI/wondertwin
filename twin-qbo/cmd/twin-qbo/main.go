// twin-qbo is a WonderTwin twin that simulates the QuickBooks Online Accounting API v3.
// It implements the core accounting workflow: chart of accounts, customers, vendors,
// invoices, bills, payments, journal entries, and financial reports.
//
// Key behavioral differences from real QBO that the twin faithfully simulates:
// - POST for both create and update (Id field presence distinguishes)
// - SyncToken optimistic locking on all entities
// - Balance-derived invoice/bill status (no explicit state machine)
// - SQL-like query endpoint (SELECT * FROM Invoice WHERE ...)
// - realmId in URL path (/v3/company/{realmId}/...)
// - Fault error envelope
// - HMAC-SHA256 webhook signing via intuit-signature header
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twinkit/telemetry"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	pkgwebhook "github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/hooks"
	"github.com/wondertwin-ai/wondertwin/twin-qbo/internal/store"
	qbowh "github.com/wondertwin-ai/wondertwin/twin-qbo/internal/webhook"
)

func main() {
	cfg := twincore.ParseFlags("twin-qbo")
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
		TwinName:     "qbo",
		TwinVersion:  "0.3.0",
	})
	if telemetryEnabled {
		emitter.Start()
		defer emitter.Stop()
	}

	// Webhook verifier token from env or default.
	verifierToken := os.Getenv("QBO_WEBHOOK_VERIFIER")
	if verifierToken == "" {
		verifierToken = "qbo_sim_test_verifier"
	}

	// Webhook dispatcher with QBO HMAC-SHA256 signing.
	dispatcher := pkgwebhook.NewDispatcher(pkgwebhook.Config{
		URL:         cfg.WebhookURL,
		Secret:      verifierToken,
		Signer:      qbowh.NewQBOSigner(),
		Logger:      twin.Logger,
		EventPrefix: "evt",
		AutoDeliver: cfg.WebhookURL != "",
	})

	// Accounting engine with QBO hooks + telemetry bridge.
	qboHooks := &hooks.QBOHooks{Dispatcher: dispatcher}
	j := journal.New(memStore.Clock)
	engine := ledger.NewEngine(
		ledger.WithJournal(j),
		ledger.WithHooks(qboHooks),
		ledger.WithTelemetry(emitter),
		ledger.WithClock(memStore.Clock),
	)

	// API handlers.
	apiHandler := api.NewHandler(memStore, engine, dispatcher, twin.Middleware(), emitter)
	apiHandler.Routes(twin.Router)

	// Admin control plane.
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetFlusher(dispatcher)
	adminHandler.SetConfigProvider(twin)
	adminHandler.Routes(twin.Router)

	// Load seed data if provided.
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

	twin.Logger.Info("twin-qbo ready",
		"port", cfg.Port,
		"webhook_url", cfg.WebhookURL,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
