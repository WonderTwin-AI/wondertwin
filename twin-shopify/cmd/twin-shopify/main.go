// twin-shopify is a WonderTwin twin that simulates the Shopify Admin REST API.
// It handles products, orders, customers, inventory, fulfillments, collections,
// webhooks, metafields, themes, and more.
//
// SDK compatibility target: github.com/bold-commerce/go-shopify
// Integration method: Override base URL
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-shopify/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-shopify")
	if cfg.Port == 0 {
		cfg.Port = 4120
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	apiHandler := api.NewHandler(memStore, twin.Middleware())
	apiHandler.Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
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

	twin.Logger.Info("twin-shopify ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
