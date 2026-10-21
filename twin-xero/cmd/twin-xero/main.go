// twin-xero is a WonderTwin app emulator for the Xero Accounting API.
//
// SDK compatibility target: xero-python 15.2.0 (primary), xero-node 20.0.0
// Integration method: override the API and identity hosts
//
// LocalStack starts it as `twin-xero --port=N` and treats it as ready when
// GET /admin/health answers {"status":"ok"}.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func main() {
	strict := flag.Bool("strict-auth", false, "refuse bearer and refresh tokens this emulator did not issue, as Xero does")
	cfg := twincore.ParseFlags("twin-xero")
	if cfg.Port == 0 {
		cfg.Port = 4115
	}

	twin := twincore.New(cfg)
	memStore := store.New()
	memStore.Rand = twin.Rand
	memStore.StrictAuth.Store(*strict)

	apiHandler := api.NewHandler(memStore, twin.Middleware())
	apiHandler.Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(api.NewConfig(twin, memStore))
	adminHandler.SetFlusher(apiHandler.Dispatcher())
	adminHandler.Routes(twin.Router)

	adminPlane := chi.NewRouter()
	adminHandler.Routes(adminPlane)
	api.MountControlAliases(twin.Router, adminPlane)

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

	twin.Logger.Info("twin-xero ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
