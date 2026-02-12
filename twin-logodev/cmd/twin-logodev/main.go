package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/pkg/admin"
	"github.com/wondertwin-ai/wondertwin/pkg/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-logodev/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-logodev")
	if cfg.Port == 0 {
		cfg.Port = 12116
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	apiHandler := api.NewHandler(memStore)
	apiHandler.Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
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
