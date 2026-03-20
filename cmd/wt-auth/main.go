// wt-auth is the WonderTwin authorization service. It validates org
// credentials, issues JIT tokens for twin binaries, and gates access
// to the Content API.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/api"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/store"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-auth/internal/token"
)

func main() {
	port := flag.Int("port", 4300, "HTTP listen port")
	dbPath := flag.String("db", "", "Path to LibSQL database file")
	flag.Parse()

	// Required env vars — fail fast.
	signingKey := os.Getenv("WT_AUTH_SIGNING_KEY")
	if signingKey == "" {
		log.Fatal("WT_AUTH_SIGNING_KEY is required")
	}
	internalKey := os.Getenv("WT_AUTH_INTERNAL_KEY")
	if internalKey == "" {
		log.Fatal("WT_AUTH_INTERNAL_KEY is required")
	}

	// Database.
	if *dbPath == "" {
		if v := os.Getenv("WT_AUTH_DB_PATH"); v != "" {
			*dbPath = v
		} else {
			*dbPath = "wt-auth.db"
		}
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	licenses := store.NewLicenseStore(db)
	tokens := store.NewTokenStore(db)
	signer := token.NewSigner(signingKey)

	handler := api.NewHandler(licenses, tokens, signer, internalKey)

	r := chi.NewRouter()
	handler.Routes(r)

	// Override port from env.
	if p := os.Getenv("WT_AUTH_PORT"); p != "" {
		fmt.Sscanf(p, "%d", port)
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("wt-auth listening on %s (db: %s)", addr, *dbPath)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
