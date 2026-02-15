// twin-clerk is a WonderTwin twin that simulates the Clerk auth API.
// It implements the subset of Clerk's Backend API used for authentication,
// with JSON request/response parsing compatible with clerk-sdk-go/v2.
//
// CRITICAL: This twin generates valid JWTs and exposes a JWKS endpoint so that
// auth middleware (which calls jwt.Verify()) works correctly.
//
// SDK compatibility target: github.com/clerk/clerk-sdk-go/v2
// Integration method: CLERK_API_URL env var
// Default port: 12112
package main

import (
	"log"
	"os"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-clerk/internal/store"
)

func main() {
	cfg := twincore.ParseFlags("twin-clerk")
	if cfg.Port == 0 {
		cfg.Port = 12112
	}

	twin := twincore.New(cfg)
	memStore := store.New()

	// JWT manager with RSA keypair for signing tokens
	jwtMgr, err := api.NewJWTManager()
	if err != nil {
		log.Fatalf("failed to initialize JWT manager: %v", err)
	}

	// API handlers
	apiHandler := api.NewHandler(memStore, twin.Middleware(), jwtMgr)
	apiHandler.Routes(twin.Router)

	// Admin control plane (shared with all twins)
	adminHandler := admin.NewHandler(memStore, twin.Middleware(), memStore.Clock)
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

	twin.Logger.Info("twin-clerk ready",
		"port", cfg.Port,
		"jwks_endpoint", "/.well-known/jwks.json",
	)

	if err := twin.Serve(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
