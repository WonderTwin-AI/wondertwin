// twin-stripe is the Stripe app emulator. It serves Stripe's /v1 API on the
// dahlia release, in the shape of 2026-08-26.dahlia, with form-encoded
// requests and JSON responses as the official SDKs expect.
//
// SDK conformance targets: stripe-go v86, stripe-node v22 and stripe-python
// v15, each pinned to 2026-08-26.dahlia (see sdk-smoke/).
//
// Configuration:
//
//	--port=N              listen port
//	STRIPE_API_VERSION    the account default API version (a dahlia version,
//	                      default 2026-08-26.dahlia)
//	STRIPE_WEBHOOK_SECRET signing secret for --webhook-url deliveries
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	stripewh "github.com/wondertwin-ai/wondertwin/twin-stripe/internal/webhook"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	pkgwebhook "github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

func main() {
	cfg := twincore.ParseFlags("twin-stripe")
	if cfg.Port == 0 {
		cfg.Port = 4111
	}

	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.Rand = twin.Rand

	// Webhook secret from env or default
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret == "" {
		webhookSecret = "whsec_sim_test_secret"
	}

	// Webhook dispatcher with Stripe v1 signing
	dispatcher := pkgwebhook.NewDispatcher(pkgwebhook.Config{
		URL:         cfg.WebhookURL,
		Secret:      webhookSecret,
		Signer:      stripewh.NewStripeSigner(memStore.Clock),
		Logger:      twin.Logger,
		EventPrefix: "evt",
		AutoDeliver: cfg.WebhookURL != "",
	})

	// Wire MemoryStore as endpoint provider for multi-endpoint webhook delivery
	dispatcher.SetEndpointProvider(memStore)

	// API handlers
	apiHandler := api.NewHandler(memStore, dispatcher, twin.Middleware())
	if v := os.Getenv("STRIPE_API_VERSION"); v != "" {
		if err := apiHandler.SetDefaultAPIVersion(v); err != nil {
			log.Fatalf("STRIPE_API_VERSION: %v", err)
		}
	}
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetFlusher(dispatcher)
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

	twin.Logger.Info("twin-stripe ready",
		"port", cfg.Port,
		"webhook_url", cfg.WebhookURL,
		"webhook_secret", webhookSecret[:10]+"...",
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
