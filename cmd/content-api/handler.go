package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// handleGetContent serves GET /v1/content/{spec} where spec = {twin}@{version}.
func handleGetContent(w http.ResponseWriter, r *http.Request) {
	spec := chi.URLParam(r, "spec")

	twin, version, ok := parseSpec(spec)
	if !ok {
		http.Error(w, `{"error":"invalid spec format, expected {twin}@{version}"}`, http.StatusBadRequest)
		return
	}

	data, err := LoadFixture(twin, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// parseSpec splits "twin@version" into its components.
func parseSpec(spec string) (twin, version string, ok bool) {
	i := strings.LastIndex(spec, "@")
	if i <= 0 || i >= len(spec)-1 {
		return "", "", false
	}
	return spec[:i], spec[i+1:], true
}
