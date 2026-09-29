package store

import "encoding/json"

// MarshalJSON renders discounts as an empty array rather than null, as the
// dahlia schema declares it a non-nullable array.
func (s Subscription) MarshalJSON() ([]byte, error) {
	type alias Subscription
	if s.Discounts == nil {
		s.Discounts = []string{}
	}
	return json.Marshal(alias(s))
}

// MarshalJSON renders discounts as an empty array rather than null, as the
// dahlia schema declares it a non-nullable array.
func (inv Invoice) MarshalJSON() ([]byte, error) {
	type alias Invoice
	if inv.Discounts == nil {
		inv.Discounts = []string{}
	}
	return json.Marshal(alias(inv))
}
