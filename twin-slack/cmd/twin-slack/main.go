// twin-slack is a WonderTwin twin that simulates the Slack Web API.
// It handles chat.postMessage, conversations.*, users.*, reactions.*,
// files.*, pins.*, and other Slack Web API methods.
//
// SDK compatibility target: github.com/slack-go/slack, @slack/web-api
// Integration method: Override base URL
//
// Events API delivery: --webhook-url is the app's Event Subscriptions Request
// URL, and SLACK_SIGNING_SECRET its signing secret. Both can also be set at
// runtime through POST /admin/events/config.
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func main() {
	cfg := twincore.ParseFlags("twin-slack")
	if cfg.Port == 0 {
		cfg.Port = 4118
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware())
	apiHandler.ConfigureEvents(api.EventsConfig{
		RequestURL:    cfg.WebhookURL,
		SigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
	})
	apiHandler.Routes(twin.Router)

	// Admin control plane
	adminHandler := admin.NewHandler(apiHandler.AdminState(), twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.SetFlusher(apiHandler.EventsFlusher())
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

	twin.Logger.Info("twin-slack ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
