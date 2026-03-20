// wt-collector is the WonderTwin telemetry collector service.
// It receives anonymized behavioral observations from twin binaries
// and stores them for the intelligence pipeline.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/api"
	"github.com/wondertwin-ai/wondertwin/cmd/wt-collector/internal/store"
)

func main() {
	port := flag.Int("port", 4400, "HTTP listen port")
	dbPath := flag.String("db", "", "Path to LibSQL database file")
	flag.Parse()

	ingestKey := os.Getenv("WT_COLLECTOR_INGEST_KEY")
	if ingestKey == "" {
		log.Fatal("WT_COLLECTOR_INGEST_KEY is required")
	}

	if *dbPath == "" {
		if v := os.Getenv("WT_COLLECTOR_DB_PATH"); v != "" {
			*dbPath = v
		} else {
			*dbPath = "wt-collector.db"
		}
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	events := store.NewEventStore(db)
	handler := api.NewHandler(events, ingestKey)

	r := chi.NewRouter()
	handler.Routes(r)

	if p := os.Getenv("WT_COLLECTOR_PORT"); p != "" {
		fmt.Sscanf(p, "%d", port)
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("wt-collector listening on %s (db: %s)", addr, *dbPath)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
