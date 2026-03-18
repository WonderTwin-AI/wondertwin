// content-api is a standalone HTTP server that serves WonderTwin intelligence
// (quirks, workflows, compatibility data) behind license-key authentication.
//
// Usage:
//
//	content-api [-port 8080] [-data-dir ./fixtures]
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	port := flag.Int("port", 8080, "HTTP listen port")
	flag.StringVar(&dataDir, "data-dir", "", "directory containing content fixtures (overrides embedded data)")
	flag.Parse()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health endpoint (unauthenticated)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Content API (authenticated)
	r.Route("/v1", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/content/{spec}", handleGetContent)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("content-api listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
