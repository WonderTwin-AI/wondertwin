package seed

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

func TestLoadInstitutionsHasPlaidsPublishedSandboxList(t *testing.T) {
	s := store.New()
	LoadInstitutions(s)
	for id, name := range map[string]string{"ins_109508": "First Platypus Bank", "ins_43": "Tartan-Dominion Bank of Canada",
		"ins_117650": "Royal Bank of Plaid", "ins_127287": "Platypus OAuth Bank", "ins_109512": "Houndstooth Bank"} {
		in, ok := s.GetInstitution(id)
		if !ok || in.Name != name {
			t.Errorf("%s = %+v, found %v", id, in, ok)
		}
	}
	for _, in := range s.ListInstitutions() {
		if in.ID == "" || in.Name == "" || len(in.CountryCodes) == 0 || len(in.Products) == 0 {
			t.Errorf("incomplete institution %+v", in)
		}
	}
	if s.Institutions.Count() != 22 {
		t.Errorf("%d institutions, want 22", s.Institutions.Count())
	}
}
