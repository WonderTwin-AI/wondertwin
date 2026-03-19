// twin-xero is a WonderTwin twin that simulates the Xero Accounting API.
// It implements the core accounting workflow: chart of accounts, invoices,
// bills, credit notes, payments, manual journals, and financial reports.
//
// SDK compatibility target: github.com/XeroAPI/xerogolang (or any HTTP client)
// Integration method: Override base URL to point at twin
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/ledger/accounting"
	"github.com/wondertwin-ai/wondertwin/twinkit/state/journal"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	pkgwebhook "github.com/wondertwin-ai/wondertwin/twinkit/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/hooks"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	xerowh "github.com/wondertwin-ai/wondertwin/twin-xero/internal/webhook"
)

func main() {
	cfg := twincore.ParseFlags("twin-xero")
	if cfg.Port == 0 {
		cfg.Port = 4115
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// Webhook secret from env or default.
	webhookSecret := os.Getenv("XERO_WEBHOOK_KEY")
	if webhookSecret == "" {
		webhookSecret = "xero_sim_test_key"
	}

	// Webhook dispatcher with Xero HMAC-SHA256 signing.
	dispatcher := pkgwebhook.NewDispatcher(pkgwebhook.Config{
		URL:         cfg.WebhookURL,
		Secret:      webhookSecret,
		Signer:      xerowh.NewXeroSigner(),
		Logger:      twin.Logger,
		EventPrefix: "evt",
		AutoDeliver: cfg.WebhookURL != "",
	})

	// Accounting engine with Xero hooks.
	xeroHooks := &hooks.XeroHooks{Dispatcher: dispatcher}
	j := journal.New(memStore.Clock)
	engine := accounting.NewEngine(
		accounting.WithJournal(j),
		accounting.WithHooks(xeroHooks),
		accounting.WithClock(memStore.Clock),
	)

	// API handlers.
	apiHandler := api.NewHandler(memStore, engine, dispatcher, twin.Middleware())
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

	twin.Logger.Info("twin-xero ready",
		"port", cfg.Port,
		"webhook_url", cfg.WebhookURL,
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
