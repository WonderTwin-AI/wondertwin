package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// institutionJSON is an institution: products, country_codes, routing_numbers
// and oauth are always written; url, primary_color, logo and dtc_numbers are
// optional and omitted when empty, never null (quirk ENVELOPE-004).
type institutionJSON struct {
	InstitutionID  string   `json:"institution_id"`
	Name           string   `json:"name"`
	Products       []string `json:"products"`
	CountryCodes   []string `json:"country_codes"`
	RoutingNumbers []string `json:"routing_numbers"`
	DTCNumbers     []string `json:"dtc_numbers,omitempty"`
	OAuth          bool     `json:"oauth"`
	URL            string   `json:"url,omitempty"`
	PrimaryColor   string   `json:"primary_color,omitempty"`
	Logo           string   `json:"logo,omitempty"`
}

func newInstitutionJSON(in store.Institution) institutionJSON {
	return institutionJSON{InstitutionID: in.ID, Name: in.Name, Products: orEmpty(in.Products),
		CountryCodes: orEmpty(in.CountryCodes), RoutingNumbers: orEmpty(in.RoutingNumbers), DTCNumbers: in.DTCNumbers,
		OAuth: in.OAuth, URL: in.URL, PrimaryColor: in.PrimaryColor, Logo: in.Logo}
}

// asInt reads a whole number from a decoded body. present is false when the
// field is absent; ok is false when it is there and is not a whole number.
func asInt(v any) (n int, present, ok bool) {
	if v == nil {
		return 0, false, true
	}
	num, isNum := v.(json.Number)
	if !isNum {
		return 0, true, false
	}
	i, err := num.Int64()
	return int(i), true, err == nil
}

// matching returns the institutions, in id order, that serve one of the
// countries and, when given, support every product and match oauth.
func (h *Handler) matching(countries, products []string, oauth *bool) []store.Institution {
	var out []store.Institution
	for _, in := range h.store.ListInstitutions() {
		if !slices.ContainsFunc(in.CountryCodes, func(c string) bool { return slices.Contains(countries, c) }) ||
			(oauth != nil && in.OAuth != *oauth) {
			continue
		}
		if slices.ContainsFunc(products, func(p string) bool { return !slices.Contains(in.Products, p) }) {
			continue
		}
		out = append(out, in)
	}
	return out
}

func (h *Handler) writeInstitutions(w http.ResponseWriter, list []store.Institution, extra map[string]any) {
	rows := make([]institutionJSON, 0, len(list))
	for _, in := range list {
		rows = append(rows, newInstitutionJSON(in))
	}
	extra["institutions"] = rows
	h.writeJSON(w, http.StatusOK, extra)
}

// options reads the optional filters shared by get and search.
func options(body map[string]any) (products []string, oauth *bool, ok bool) {
	opts, isObj := body["options"].(map[string]any)
	if body["options"] != nil && !isObj {
		return nil, nil, false
	}
	products, okP := asStrings(opts["products"])
	if v, set := opts["oauth"]; set {
		b, isBool := v.(bool)
		if !isBool {
			return nil, nil, false
		}
		oauth = &b
	}
	return products, oauth, okP
}

// institutionsGet is POST /institutions/get: institutions in id order, paged by
// count and a 0-based offset (quirk PAGINATION-003).
func (h *Handler) institutionsGet(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	count, hasCount, ok1 := asInt(call.Body["count"])
	offset, hasOffset, ok2 := asInt(call.Body["offset"])
	countries, ok3 := asStrings(call.Body["country_codes"])
	products, oauth, ok4 := options(call.Body)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		h.invalidBody(w)
		return
	}
	var missing []string
	for _, f := range []struct {
		field  string
		absent bool
	}{{"count", !hasCount}, {"offset", !hasOffset}, {"country_codes", len(countries) == 0}} {
		if f.absent {
			missing = append(missing, f.field)
		}
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	if count < 1 || count > 500 {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "count must be between 1 and 500")
		return
	}
	if offset < 0 {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "INVALID_FIELD", "offset must be 0 or greater")
		return
	}
	all := h.matching(countries, products, oauth)
	end := min(offset+count, len(all))
	page := []store.Institution{}
	if offset < len(all) {
		page = all[offset:end]
	}
	h.writeInstitutions(w, page, map[string]any{"total": len(all)})
}

// institutionsSearch is POST /institutions/search: up to ten institutions
// whose name contains the query.
func (h *Handler) institutionsSearch(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	query, ok1 := asString(call.Body["query"])
	countries, ok2 := asStrings(call.Body["country_codes"])
	products, ok3 := asStrings(call.Body["products"])
	_, oauth, ok4 := options(call.Body)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		h.invalidBody(w)
		return
	}
	var missing []string
	if query == "" {
		missing = append(missing, "query")
	}
	if len(countries) == 0 {
		missing = append(missing, "country_codes")
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	var found []store.Institution
	for _, in := range h.matching(countries, products, oauth) {
		if strings.Contains(strings.ToLower(in.Name), strings.ToLower(query)) && len(found) < 10 {
			found = append(found, in)
		}
	}
	h.writeInstitutions(w, found, map[string]any{})
}

// institutionsGetByID is POST /institutions/get_by_id.
func (h *Handler) institutionsGetByID(w http.ResponseWriter, r *http.Request) {
	call, _ := CallFrom(r)
	id, ok1 := asString(call.Body["institution_id"])
	countries, ok2 := asStrings(call.Body["country_codes"])
	if !ok1 || !ok2 {
		h.invalidBody(w)
		return
	}
	var missing []string
	if id == "" {
		missing = append(missing, "institution_id")
	}
	if len(countries) == 0 {
		missing = append(missing, "country_codes")
	}
	if len(missing) > 0 {
		h.writeMissing(w, missing)
		return
	}
	in, found := h.store.GetInstitution(id)
	if !found {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "INVALID_INSTITUTION", "invalid institution_id provided")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"institution": newInstitutionJSON(in)})
}
