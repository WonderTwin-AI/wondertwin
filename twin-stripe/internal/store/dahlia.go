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

// MarshalJSON renders an invoice's lines with data as an empty array rather
// than null when the invoice has no lines. Webhook deliveries are encoded from
// the store type without the response fill, so this is where they get it.
func (l InvoiceLines) MarshalJSON() ([]byte, error) {
	type alias InvoiceLines
	if l.Data == nil {
		l.Data = []InvoiceLine{}
	}
	return json.Marshal(alias(l))
}
