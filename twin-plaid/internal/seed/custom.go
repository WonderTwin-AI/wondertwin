package seed

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"
)

// defaultRouting is the fixture checking account's routing number, from the
// Sandbox institutions page (Houndstooth Bank).
const defaultRouting = "021000021"

// authSubtypes are the depository subtypes Auth serves numbers for.
var authSubtypes = []string{"checking", "savings", "cash management"}

// CustomAccounts builds the accounts of a user_custom Item, one per
// override_accounts entry, in order. Ids, Item and Position are the caller's.
//
// What the configuration leaves out comes from the fixture, not from Plaid's
// random generator, and seed is ignored (declared in divergences.json):
//   - name, official name, mask and balances take the fixture account of the
//     same subtype, else of the same type, else Plaid Checking; a fixture name
//     is renamed for a subtype the fixture lacks ("Plaid Savings");
//   - a missing mask is the last four of numbers.account, else the fixture's,
//     else its position repeated: 1111 for the first account, 2222 for the second;
//   - a checking, savings or cash management account without numbers gets
//     routing 021000021 and an account number ending in its mask, so /auth/get
//     serves it (a labelled choice).
//
// balances.current is starting_balance: the current balance at Item creation,
// fixed afterwards, as the declared balances divergence says (quirk
// LIFECYCLE-007, operator decision 2026-11-03). available is
// force_available_balance, null with has_null_available_balance, and otherwise
// current. Every account gets the fixture's account holder.
func CustomAccounts(cfg *store.CustomUser) []store.Account {
	fixtures := DefaultAccounts()
	out := make([]store.Account, 0, len(cfg.OverrideAccounts))
	for i, c := range cfg.OverrideAccounts {
		f := fixtureFor(fixtures, c.Type, c.Subtype)
		a := store.Account{Type: c.Type, Subtype: c.Subtype, Owners: f.Owners}

		a.Name, a.OfficialName = c.Meta.Name, c.Meta.OfficialName
		if a.Name == "" {
			a.Name = f.Name
			if f.Subtype != c.Subtype {
				a.Name = "Plaid " + titleCase(c.Subtype)
			}
			if a.OfficialName == "" && f.Subtype == c.Subtype {
				a.OfficialName = f.OfficialName
			}
		}
		switch {
		case c.Meta.Mask != nil:
			a.Mask = *c.Meta.Mask
		case len(c.Numbers.Account) >= 4:
			a.Mask = c.Numbers.Account[len(c.Numbers.Account)-4:]
		case f.Subtype == c.Subtype:
			a.Mask = f.Mask
		default:
			a.Mask = strings.Repeat(fmt.Sprint((i%9)+1), 4)
		}

		currency := c.Currency
		if currency == "" {
			currency = "USD"
		}
		a.Balances = store.Balances{ISOCurrencyCode: currency, Limit: c.Meta.Limit}
		if c.StartingBalance != nil {
			current, available := *c.StartingBalance, *c.StartingBalance
			a.Balances.Current, a.Balances.Available = &current, &available
		} else {
			a.Balances.Current, a.Balances.Available = f.Balances.Current, f.Balances.Available
		}
		switch {
		case c.ForceAvailableBalance != nil:
			a.Balances.Available = c.ForceAvailableBalance
		case c.HasNullAvailableBalance:
			a.Balances.Available = nil
		}

		if c.Type == "depository" && slices.Contains(authSubtypes, c.Subtype) {
			n := store.AccountNumbers{Account: c.Numbers.Account, Routing: c.Numbers.ACHRouting, WireRouting: c.Numbers.ACHWireRouting}
			if n.Account == "" {
				n.Account = "111122223333" + a.Mask
			}
			if n.Routing == "" {
				n.Routing = defaultRouting
			}
			a.Numbers = &n
		}
		out = append(out, a)
	}
	return out
}

// fixtureFor picks the fixture account a configured account borrows its
// missing values from.
func fixtureFor(fixtures []store.Account, typ, subtype string) store.Account {
	for _, f := range fixtures {
		if f.Type == typ && f.Subtype == subtype {
			return f
		}
	}
	for _, f := range fixtures {
		if f.Type == typ {
			return f
		}
	}
	return fixtures[0]
}

// titleCase capitalises each word: "credit card" is "Credit Card".
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
