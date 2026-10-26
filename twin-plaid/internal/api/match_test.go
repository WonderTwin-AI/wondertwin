package api

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

var holder = []store.Owner{{
	Names:        []string{"Alberta Bobbeth Charleson"},
	PhoneNumbers: []store.OwnerPhone{{Data: "2025550123"}},
	Emails:       []store.OwnerEmail{{Data: "holder@example.com"}},
	Addresses:    []store.OwnerAddress{{Data: store.AddressData{Street: "2992 Cameron Road", City: "Malakoff", Region: "NY", PostalCode: "93405-2255"}}},
}}

func TestScoresLandInPlaidsDocumentedBands(t *testing.T) {
	for name, tc := range map[string]struct{ got, lo, hi int }{
		"name exact":         {*scoreName("Alberta Bobbeth Charleson", holder).Score, 100, 100},
		"name any order":     {*scoreName("Charleson Alberta Bobbeth", holder).Score, 85, 99},
		"name no middle":     {*scoreName("Alberta Charleson", holder).Score, 85, 99},
		"name first only":    {*scoreName("Alberta Smith", holder).Score, 70, 84},
		"name different":     {*scoreName("Jane Smith", holder).Score, 0, 69},
		"phone formatted":    {*scorePhone("+1 (202) 555-0123", holder).Score, 100, 100},
		"phone different":    {*scorePhone("415 555 2671", holder).Score, 0, 69},
		"email case":         {*scoreEmail("HOLDER@example.com", holder).Score, 100, 100},
		"email plus tag":     {*scoreEmail("holder+tax@example.com", holder).Score, 70, 99},
		"email different":    {*scoreEmail("other@example.com", holder).Score, 0, 69},
		"address zip plus 4": {*scoreAddress(store.AddressData{Street: "2992 Cameron Rd.", City: "Malakoff", Region: "ny", PostalCode: "93405"}, holder).Score, 70, 99},
		"address postal":     {*scoreAddress(store.AddressData{Street: "1 Elsewhere St", PostalCode: "93405"}, holder).Score, 70, 89},
		"address different":  {*scoreAddress(store.AddressData{Street: "1 Elsewhere St", PostalCode: "10001"}, holder).Score, 0, 69},
	} {
		if tc.got < tc.lo || tc.got > tc.hi {
			t.Errorf("%s = %d, want %d to %d", name, tc.got, tc.lo, tc.hi)
		}
	}
}

func TestScoresAreNullWhenEitherSideHasNoData(t *testing.T) {
	if scoreName("", holder).Score != nil || scoreName("Alberta", nil).Score != nil ||
		scorePhone("", holder).Score != nil || scorePhone("2025550123", nil).Score != nil ||
		scoreEmail("", holder).Score != nil || scoreEmail("a@b.c", nil).Score != nil ||
		scoreAddress(store.AddressData{}, holder).Score != nil || scoreAddress(store.AddressData{Street: "x"}, nil).Score != nil {
		t.Fatal("a score was given without data on both sides")
	}
}

func TestMoreThanOneOwnerTakesTheBestScore(t *testing.T) {
	owners := append([]store.Owner{{Names: []string{"Jane Smith"}}}, holder...)
	if got := *scoreName("Alberta Bobbeth Charleson", owners).Score; got != 100 {
		t.Fatalf("best of two owners = %d", got)
	}
}
