// twin-github is the community app emulator for the GitHub REST API at
// calendar version 2026-03-10. It serves repositories, contents and git data,
// issues, pull requests and reviews, commit statuses and check runs, GitHub
// App installation tokens, releases, workflow dispatch and signed webhook
// delivery.
//
// SDK targets: Octokit.js (@octokit/plugin-rest-endpoint-methods 17.0.0) and
// google/go-github v92 (third party). Clients reach it by overriding the base
// URL; under lstk it is http://github.localhost.localstack.cloud:4566.
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twin-github/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-github/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

func main() {
	cfg := twincore.ParseFlags("twin-github")
	if cfg.Port == 0 {
		cfg.Port = 4119
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	apiHandler := api.NewHandler(memStore, twin.Middleware())
	apiHandler.Routes(twin.Router)

	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
	adminHandler.SetConfigProvider(twin)
	adminHandler.SetFlusher(apiHandler)
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

	twin.Logger.Info("twin-github ready", "port", cfg.Port)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
