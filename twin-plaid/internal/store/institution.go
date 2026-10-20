package store

import "sort"

// Institution is a bank the emulator can link. Ids are arbitrary, as in
// Plaid's Sandbox (ins_109508, ins_56): they arrive through seed data, not
// from a generator (quirk IDENTITY-001).
type Institution struct {
	ID             string   `json:"institution_id"`
	Name           string   `json:"name"`
	Products       []string `json:"products"`
	CountryCodes   []string `json:"country_codes"`
	RoutingNumbers []string `json:"routing_numbers"`
	OAuth          bool     `json:"oauth"`
	URL            string   `json:"url,omitempty"`
	PrimaryColor   string   `json:"primary_color,omitempty"`
	Logo           string   `json:"logo,omitempty"`
}

// PutInstitution stores an institution under its ID.
func (s *MemoryStore) PutInstitution(in Institution) { s.Institutions.Set(in.ID, in) }

// GetInstitution returns the institution with the given ID.
func (s *MemoryStore) GetInstitution(id string) (Institution, bool) { return s.Institutions.Get(id) }

// ListInstitutions returns every institution in ID order.
func (s *MemoryStore) ListInstitutions() []Institution {
	out := s.Institutions.List()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
