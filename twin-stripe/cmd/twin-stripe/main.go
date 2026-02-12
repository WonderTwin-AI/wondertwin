// twin-stripe is a WonderTwin twin that simulates the Stripe Connect API.
// It implements the subset of Stripe's API used for settlement,
// with form-encoded request parsing and JSON responses compatible with stripe-go/v76.
//
// SDK compatibility target: github.com/stripe/stripe-go/v76
// Integration method: stripe.SetBackend() to override API URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/pkg/admin"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	pkgwebhook "github.com/wondertwin-ai/wondertwin/pkg/webhook"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
	stripewh "github.com/wondertwin-ai/wondertwin/twin-stripe/internal/webhook"
)

func main() {
	cfg := twincore.ParseFlags("twin-stripe")
	if cfg.Port == 0 {
		cfg.Port = 12111
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// Webhook secret from env or default
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret == "" {
		webhookSecret = "whsec_sim_test_secret"
	}

	// Webhook dispatcher with Stripe v1 signing
	dispatcher := pkgwebhook.NewDispatcher(pkgwebhook.Config{
		URL:         cfg.WebhookURL,
		Secret:      webhookSecret,
		Signer:      stripewh.NewStripeSigner(),
		Logger:      twin.Logger,
		EventPrefix: "evt",
		AutoDeliver: cfg.WebhookURL != "",
	})

	// API handlers
	apiHandler := api.NewHandler(memStore, dispatcher, twin.Middleware())
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetFlusher(dispatcher)
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
