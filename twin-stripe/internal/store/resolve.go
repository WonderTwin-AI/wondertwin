package store

import (
	"encoding/json"
	"strings"
)

// Resolve implements expand.Resolver: it looks up id by its Stripe-style
// prefix (the segment before the first underscore, e.g. "cus" in
// "cus_000001") and returns the matching object serialized the same way it
// would be in an ordinary API response.
//
// This is the mechanical, permissive half of expansion support: it
// resolves any known ID regardless of which field it was found in or
// whether real Stripe's API would allow expanding that field on that
// endpoint. Callers that need Stripe's exact per-endpoint expandable-field
// contract enforce an allowlist before calling expand.Apply with this
// resolver.
func (s *MemoryStore) Resolve(id string) (map[string]any, bool) {
	prefix, _, ok := strings.Cut(id, "_")
	if !ok {
		return nil, false
	}

	switch prefix {
	case "acct":
		return resolveFrom(s.Accounts, id)
	case "ba":
		return resolveFrom(s.ExternalAccts, id)
	case "tr":
		return resolveFrom(s.Transfers, id)
	case "po":
		return resolveFrom(s.Payouts, id)
	case "evt":
		return resolveFrom(s.Events, id)
	case "txn":
		return resolveFrom(s.BalanceTransactions, id)
	case "cus":
		return resolveFrom(s.Customers, id)
	case "prod":
		return resolveFrom(s.Products, id)
	case "price":
		return resolveFrom(s.Prices, id)
	case "pi":
		return resolveFrom(s.PaymentIntents, id)
	case "pm":
		return resolveFrom(s.PaymentMethods, id)
	case "ch":
		return resolveFrom(s.Charges, id)
	case "re":
		return resolveFrom(s.Refunds, id)
	case "sub":
		return resolveFrom(s.Subscriptions, id)
	case "in":
		return resolveFrom(s.Invoices, id)
	case "ii":
		return resolveFrom(s.InvoiceItems, id)
	case "coup":
		return resolveFrom(s.Coupons, id)
	case "seti":
		return resolveFrom(s.SetupIntents, id)
	case "txr":
		return resolveFrom(s.TaxRates, id)
	case "dp":
		return resolveFrom(s.Disputes, id)
	case "cs":
		return resolveFrom(s.CheckoutSessions, id)
	case "plink":
		return resolveFrom(s.PaymentLinks, id)
	case "tok":
		return resolveFrom(s.Tokens, id)
	case "src":
		return resolveFrom(s.Sources, id)
	case "mandate":
		return resolveFrom(s.Mandates, id)
	case "ctoken":
		return resolveFrom(s.ConfirmationTokens, id)
	case "cn":
		return resolveFrom(s.CreditNotes, id)
	case "promo":
		return resolveFrom(s.PromotionCodes, id)
	case "si":
		return resolveFrom(s.SubItems, id)
	case "qt":
		return resolveFrom(s.Quotes, id)
	case "bps":
		return resolveFrom(s.BillingPortalSessions, id)
	case "prv":
		return resolveFrom(s.Reviews, id)
	case "txi":
		return resolveFrom(s.TaxIDs, id)
	case "we":
		return resolveFrom(s.WebhookEndpoints, id)
	case "file":
		return resolveFrom(s.Files, id)
	case "link":
		return resolveFrom(s.FileLinks, id)
	case "shr":
		return resolveFrom(s.ShippingRates, id)
	case "fee":
		return resolveFrom(s.ApplicationFees, id)
	case "fr":
		return resolveFrom(s.ApplicationFeeRefunds, id)
	case "trr":
		return resolveFrom(s.TransferReversals, id)
	case "person":
		return resolveFrom(s.Persons, id)
	case "tu":
		return resolveFrom(s.TopUps, id)
	default:
		return nil, false
	}
}

// getter is satisfied by *pkgstate.Store[T] for any T; it lets resolveFrom
// stay generic over the concrete resource type.
type getter[T any] interface {
	Get(id string) (T, bool)
}

func resolveFrom[T any](store getter[T], id string) (map[string]any, bool) {
	item, ok := store.Get(id)
	if !ok {
		return nil, false
	}
	b, err := json.Marshal(item)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}
