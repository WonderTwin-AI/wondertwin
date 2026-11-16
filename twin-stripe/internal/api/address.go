package api

import (
	"net/http"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/store"
)

var addressParts = []string{"city", "country", "line1", "line2", "postal_code", "state"}

// formAddress reads an address sent as prefix[city], prefix[country] and so on.
// set reports whether any part was sent; an empty prefix value clears it.
func formAddress(r *http.Request, prefix string) (addr *store.Address, set bool) {
	if v, ok := r.Form[prefix]; ok && len(v) > 0 && v[0] == "" {
		return nil, true
	}
	a := &store.Address{}
	fields := map[string]**string{"city": &a.City, "country": &a.Country, "line1": &a.Line1,
		"line2": &a.Line2, "postal_code": &a.PostalCode, "state": &a.State}
	for _, part := range addressParts {
		if vals, ok := r.Form[prefix+"["+part+"]"]; ok && len(vals) > 0 {
			v := vals[0]
			*fields[part] = &v
			set = true
		}
	}
	if !set {
		return nil, false
	}
	return a, true
}

// mergeAddress applies the parts sent in update over the current address.
func mergeAddress(cur, update *store.Address) *store.Address {
	if cur == nil || update == nil {
		return update
	}
	out := *cur
	for _, p := range []struct{ dst, src **string }{{&out.City, &update.City}, {&out.Country, &update.Country},
		{&out.Line1, &update.Line1}, {&out.Line2, &update.Line2}, {&out.PostalCode, &update.PostalCode}, {&out.State, &update.State}} {
		if *p.src != nil {
			*p.dst = *p.src
		}
	}
	return &out
}

// formShipping reads shipping[name], shipping[phone] and shipping[address][...].
func formShipping(r *http.Request, cur *store.Shipping) (*store.Shipping, bool) {
	if v, ok := r.Form["shipping"]; ok && len(v) > 0 && v[0] == "" {
		return nil, true
	}
	addr, addrSet := formAddress(r, "shipping[address]")
	name, nameSet := r.Form["shipping[name]"]
	phone, phoneSet := r.Form["shipping[phone]"]
	if !addrSet && !nameSet && !phoneSet {
		return cur, false
	}
	out := &store.Shipping{}
	if cur != nil {
		*out = *cur
	}
	if addrSet {
		out.Address = mergeAddress(out.Address, addr)
	}
	if nameSet && len(name) > 0 {
		out.Name = name[0]
	}
	if phoneSet && len(phone) > 0 {
		v := phone[0]
		out.Phone = &v
	}
	return out, true
}
