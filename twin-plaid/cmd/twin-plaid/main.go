// twin-plaid is a WonderTwin app emulator for the Plaid API.
//
// SDK compatibility target: plaid-python 44.0.0 (primary), plaid-node 47.0.0, plaid-go 47.0.0
// API version: 2020-09-14, OpenAPI revision 1.740.1
// Integration method: override the API host
//
// LocalStack starts it as `twin-plaid --port=N` and treats it as ready when
// GET /admin/health answers {"status":"ok"}.
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func main() {
	cfg := twincore.ParseFlags("twin-plaid")
	if cfg.Port == 0 {
		cfg.Port = 4116
	}

	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.Rand = twin.Rand

	apiHandler := api.NewHandler(memStore, twin.Middleware())
	// PLAID_CLIENT_ID and PLAID_SECRET are enforced independently: each, when
	// set, must match; an unset one accepts any value. With neither set, any
	// non-empty pair is accepted.
	apiHandler.SetCredentials(os.Getenv("PLAID_CLIENT_ID"), os.Getenv("PLAID_SECRET"))
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

	twin.Logger.Info("twin-plaid ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
