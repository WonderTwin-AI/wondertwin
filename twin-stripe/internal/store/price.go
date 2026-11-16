package store

import "encoding/json"

// MarshalJSON answers an unset tax_behavior as unspecified, as Stripe does, so
// prices loaded from a seed or /admin/state without the key never answer "".
func (p Price) MarshalJSON() ([]byte, error) {
	type alias Price
	if p.TaxBehavior == "" {
		p.TaxBehavior = "unspecified"
	}
	return json.Marshal(alias(p))
}
