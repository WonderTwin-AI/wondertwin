package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

var addressParts = []string{"city", "country", "line1", "line2", "postal_code", "state"}

// formAddress applies an address sent as prefix[city], prefix[country] and so
// on over cur. set reports whether any part was sent; an empty prefix value
// clears the address and an empty part clears that part.
func formAddress(r *http.Request, prefix string, cur *store.Address) (addr *store.Address, set bool) {
	if v, ok := r.Form[prefix]; ok && len(v) > 0 && v[0] == "" {
		return nil, true
	}
	a := &store.Address{}
	if cur != nil {
		*a = *cur
	}
	fields := map[string]**string{"city": &a.City, "country": &a.Country, "line1": &a.Line1,
		"line2": &a.Line2, "postal_code": &a.PostalCode, "state": &a.State}
	for _, part := range addressParts {
		if vals, ok := r.Form[prefix+"["+part+"]"]; ok && len(vals) > 0 {
			*fields[part] = optString(vals[0])
			set = true
		}
	}
	if !set {
		return cur, false
	}
	return a, true
}

func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// formShipping reads shipping[name], shipping[phone] and shipping[address][...].
func formShipping(r *http.Request, cur *store.Shipping) (*store.Shipping, bool) {
	if v, ok := r.Form["shipping"]; ok && len(v) > 0 && v[0] == "" {
		return nil, true
	}
	out := &store.Shipping{}
	if cur != nil {
		*out = *cur
	}
	var addrSet bool
	out.Address, addrSet = formAddress(r, "shipping[address]", out.Address)
	name, nameSet := r.Form["shipping[name]"]
	phone, phoneSet := r.Form["shipping[phone]"]
	if !addrSet && !nameSet && !phoneSet {
		return cur, false
	}
	if nameSet && len(name) > 0 {
		out.Name = name[0]
	}
	if phoneSet && len(phone) > 0 {
		out.Phone = optString(phone[0])
	}
	return out, true
}
