package seed

import (
	_ "embed"
	"encoding/json"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

//go:embed institutions.json
var institutionsJSON []byte

// LoadInstitutions puts the institutions the emulator knows into a store:
// Plaid's published Sandbox list plus a few published sample rows. Provenance
// and what is provisional are in institutions.json.
func LoadInstitutions(s *store.MemoryStore) {
	var f struct {
		Institutions map[string]store.Institution `json:"institutions"`
	}
	if err := json.Unmarshal(institutionsJSON, &f); err != nil {
		panic("seed: institutions.json: " + err.Error())
	}
	for _, in := range f.Institutions {
		s.PutInstitution(in)
	}
}
