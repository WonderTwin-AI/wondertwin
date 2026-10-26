// Package seed holds the fixtures a new Item starts from.
package seed

import "github.com/wondertwin-ai/wondertwin/twin-plaid/internal/store"

// DefaultAccounts is the account set a new Sandbox Item starts with.
//
// PROVISIONAL. Plaid's Sandbox attaches a standard account set to a linked
// Item, and the research never captured it (RESEARCH-STATUS.md gap 4,
// WonderTwin-AI/wondertwin-docs research/twins/plaid). These three accounts,
// with their names, masks, types, subtypes and balances, are the sample in
// the POST /accounts/get response example on https://plaid.com/docs/api/accounts/
// (fetched 2026-10-20; not the pinned 2026-09-28 docs capture). The real
// default set is larger, so a credit card and the other types are missing
// until an authenticated capture replaces this. Only the checking account has
// bank numbers: that is all the Sandbox institutions page publishes. Ids are not the sample's:
// they are opaque and assigned per Item.
func DefaultAccounts() []store.Account {
	// The account holder is the one in the identity/get response example on
	// plaid.com/docs/api/products/identity, verbatim. The sample shows one holder
	// on its accounts, so every account here has the same one.
	owner := func() []store.Owner {
		return []store.Owner{{
			Names: []string{"Alberta Bobbeth Charleson"},
			PhoneNumbers: []store.OwnerPhone{{Data: "2025550123", Type: "home"}, {Data: "1112224444", Type: "work"},
				{Data: "1112225555", Type: "mobile"}},
			Emails: []store.OwnerEmail{{Data: "accountholder0@example.com", Primary: true, Type: "primary"},
				{Data: "accountholder1@example.com", Type: "secondary"},
				{Data: "extraordinarily.long.email.username.123456@reallylonghostname.com", Type: "other"}},
			Addresses: []store.OwnerAddress{
				{Data: store.AddressData{City: "Malakoff", Country: "US", PostalCode: "14236", Region: "NY", Street: "2992 Cameron Road"}, Primary: true},
				{Data: store.AddressData{City: "San Matias", Country: "US", PostalCode: "93405-2255", Region: "CA", Street: "2493 Leisure Lane"}}},
		}}
	}
	amount := func(v float64) *float64 { return &v }
	usd := func(available, current *float64) store.Balances {
		return store.Balances{Available: available, Current: current, ISOCurrencyCode: "USD"}
	}
	accounts := []store.Account{
		{Name: "Plaid Checking", OfficialName: "Plaid Gold Standard 0% Interest Checking", Mask: "0000",
			Type: "depository", Subtype: "checking", Balances: usd(amount(100), amount(110)),
			// Routing and account numbers are from the Sandbox institutions page (Houndstooth Bank,
			// Plaid Checking ****0000), which publishes them; the wire routing number is not published.
			Numbers: &store.AccountNumbers{Account: "1111222233330000", Routing: "021000021"}},
		{Name: "Plaid 401k", Mask: "6666", Type: "investment", Subtype: "401k",
			Balances: usd(nil, amount(23631.9805))},
		{Name: "Plaid Student Loan", Mask: "7777", Type: "loan", Subtype: "student",
			Balances: usd(nil, amount(65262))},
	}
	for i := range accounts {
		accounts[i].Owners = owner()
	}
	return accounts
}
