package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// MemoryStore holds all Stripe twin state in memory.
type MemoryStore struct {
	mu sync.RWMutex

	Accounts              *pkgstate.Store[Account]
	ExternalAccts         *pkgstate.Store[ExternalAccount]
	Transfers             *pkgstate.Store[Transfer]
	Payouts               *pkgstate.Store[Payout]
	Events                *pkgstate.Store[Event]
	BalanceTransactions   *pkgstate.Store[BalanceTransaction]
	Customers             *pkgstate.Store[Customer]
	Products              *pkgstate.Store[Product]
	Prices                *pkgstate.Store[Price]
	PaymentIntents        *pkgstate.Store[PaymentIntent]
	PaymentMethods        *pkgstate.Store[PaymentMethod]
	Charges               *pkgstate.Store[Charge]
	Refunds               *pkgstate.Store[Refund]
	Subscriptions         *pkgstate.Store[Subscription]
	Invoices              *pkgstate.Store[Invoice]
	InvoiceItems          *pkgstate.Store[InvoiceItem]
	Coupons               *pkgstate.Store[Coupon]
	SetupIntents          *pkgstate.Store[SetupIntent]
	TaxRates              *pkgstate.Store[TaxRate]
	Disputes              *pkgstate.Store[Dispute]
	CheckoutSessions      *pkgstate.Store[CheckoutSession]
	PaymentLinks          *pkgstate.Store[PaymentLink]
	Tokens                *pkgstate.Store[Token]
	Sources               *pkgstate.Store[Source]
	Mandates              *pkgstate.Store[Mandate]
	ConfirmationTokens    *pkgstate.Store[ConfirmationToken]
	CreditNotes           *pkgstate.Store[CreditNote]
	PromotionCodes        *pkgstate.Store[PromotionCode]
	SubItems              *pkgstate.Store[SubscriptionItem]
	Quotes                *pkgstate.Store[Quote]
	BillingPortalSessions *pkgstate.Store[BillingPortalSession]
	Reviews               *pkgstate.Store[Review]
	TaxIDs                *pkgstate.Store[TaxID]
	WebhookEndpoints      *pkgstate.Store[WebhookEndpoint]
	Files                 *pkgstate.Store[File]
	FileLinks             *pkgstate.Store[FileLink]
	ShippingRates         *pkgstate.Store[ShippingRate]
	ApplicationFees       *pkgstate.Store[ApplicationFee]
	ApplicationFeeRefunds *pkgstate.Store[ApplicationFeeRefund]
	TransferReversals     *pkgstate.Store[TransferReversal]
	Persons               *pkgstate.Store[Person]
	TopUps                *pkgstate.Store[TopUp]

	// Per-account balances (account ID -> balance)
	Balances map[string]*AccountBalance

	// Platform balance (the main Stripe account)
	PlatformBalance *AccountBalance

	Clock *pkgstate.Clock

	// Rand is the deterministic random source used by handlers for
	// token-shaped strings (whsec_, client_secret, etc.). Wired by
	// the twin's main.go to twincore.Twin.Rand so handler-level
	// randomness shares the simulator's seed.
	Rand *sim.Rand
}

// RandHex returns a deterministic hex string of n bytes (2n hex
// chars) drawn from the store's Rand. Handlers use it in place of
// crypto/rand so token-shaped output is reproducible per seed.
func (s *MemoryStore) RandHex(n int) string {
	if s == nil || s.Rand == nil || n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte(s.Rand.Intn(256))
	}
	return hex.EncodeToString(buf)
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Accounts:              pkgstate.New[Account]("acct"),
		ExternalAccts:         pkgstate.New[ExternalAccount]("ba"),
		Transfers:             pkgstate.New[Transfer]("tr"),
		Payouts:               pkgstate.New[Payout]("po"),
		Events:                pkgstate.New[Event]("evt"),
		BalanceTransactions:   pkgstate.New[BalanceTransaction]("txn"),
		Customers:             pkgstate.New[Customer]("cus"),
		Products:              pkgstate.New[Product]("prod"),
		Prices:                pkgstate.New[Price]("price"),
		PaymentIntents:        pkgstate.New[PaymentIntent]("pi"),
		PaymentMethods:        pkgstate.New[PaymentMethod]("pm"),
		Charges:               pkgstate.New[Charge]("ch"),
		Refunds:               pkgstate.New[Refund]("re"),
		Subscriptions:         pkgstate.New[Subscription]("sub"),
		Invoices:              pkgstate.New[Invoice]("in"),
		InvoiceItems:          pkgstate.New[InvoiceItem]("ii"),
		Coupons:               pkgstate.New[Coupon]("coup"),
		SetupIntents:          pkgstate.New[SetupIntent]("seti"),
		TaxRates:              pkgstate.New[TaxRate]("txr"),
		Disputes:              pkgstate.New[Dispute]("dp"),
		CheckoutSessions:      pkgstate.New[CheckoutSession]("cs"),
		PaymentLinks:          pkgstate.New[PaymentLink]("plink"),
		Tokens:                pkgstate.New[Token]("tok"),
		Sources:               pkgstate.New[Source]("src"),
		Mandates:              pkgstate.New[Mandate]("mandate"),
		ConfirmationTokens:    pkgstate.New[ConfirmationToken]("ctoken"),
		CreditNotes:           pkgstate.New[CreditNote]("cn"),
		PromotionCodes:        pkgstate.New[PromotionCode]("promo"),
		SubItems:              pkgstate.New[SubscriptionItem]("si"),
		Quotes:                pkgstate.New[Quote]("qt"),
		BillingPortalSessions: pkgstate.New[BillingPortalSession]("bps"),
		Reviews:               pkgstate.New[Review]("prv"),
		TaxIDs:                pkgstate.New[TaxID]("txi"),
		WebhookEndpoints:      pkgstate.New[WebhookEndpoint]("we"),
		Files:                 pkgstate.New[File]("file"),
		FileLinks:             pkgstate.New[FileLink]("link"),
		ShippingRates:         pkgstate.New[ShippingRate]("shr"),
		ApplicationFees:       pkgstate.New[ApplicationFee]("fee"),
		ApplicationFeeRefunds: pkgstate.New[ApplicationFeeRefund]("fr"),
		TransferReversals:     pkgstate.New[TransferReversal]("trr"),
		Persons:               pkgstate.New[Person]("person"),
		TopUps:                pkgstate.New[TopUp]("tu"),
		Balances:              make(map[string]*AccountBalance),
		PlatformBalance:       NewAccountBalance(),
		Clock:                 pkgstate.NewClock(),
	}
}

// ActiveEndpoints returns all enabled webhook endpoints, implementing webhook.EndpointProvider.
func (s *MemoryStore) ActiveEndpoints() []webhook.Endpoint {
	all := s.WebhookEndpoints.Filter(func(_ string, we WebhookEndpoint) bool {
		return we.Status == "enabled"
	})
	endpoints := make([]webhook.Endpoint, 0, len(all))
	for _, we := range all {
		endpoints = append(endpoints, webhook.Endpoint{
			URL:           we.URL,
			Secret:        we.Secret,
			EnabledEvents: we.EnabledEvents,
			Enabled:       true,
		})
	}
	return endpoints
}

// GetOrCreateBalance returns the balance for an account, creating it if needed.
func (s *MemoryStore) GetOrCreateBalance(accountID string) *AccountBalance {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.Balances[accountID]; ok {
		return b
	}
	b := NewAccountBalance()
	s.Balances[accountID] = b
	return b
}

// GetBalance returns the balance for an account or the platform balance.
func (s *MemoryStore) GetBalance(accountID string) *Balance {
	var ab *AccountBalance
	if accountID == "" {
		ab = s.PlatformBalance
	} else {
		ab = s.GetOrCreateBalance(accountID)
	}

	balance := &Balance{
		Object:   "balance",
		Livemode: false,
	}
	for currency, amount := range ab.Available {
		balance.Available = append(balance.Available, BalanceAmount{Amount: amount, Currency: currency})
	}
	for currency, amount := range ab.Pending {
		balance.Pending = append(balance.Pending, BalanceAmount{Amount: amount, Currency: currency})
	}
	if len(balance.Available) == 0 {
		balance.Available = []BalanceAmount{{Amount: 0, Currency: "usd"}}
	}
	if len(balance.Pending) == 0 {
		balance.Pending = []BalanceAmount{{Amount: 0, Currency: "usd"}}
	}
	return balance
}

// CreditBalance adds to an account's available balance.
// Pass empty accountID for the platform balance.
func (s *MemoryStore) CreditBalance(accountID string, currency string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b *AccountBalance
	if accountID == "" {
		b = s.PlatformBalance
	} else {
		if existing, ok := s.Balances[accountID]; ok {
			b = existing
		} else {
			b = NewAccountBalance()
			s.Balances[accountID] = b
		}
	}
	b.Available[currency] += amount
}

// DebitBalance subtracts from an account's available balance.
// Pass empty accountID for the platform balance.
func (s *MemoryStore) DebitBalance(accountID string, currency string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b *AccountBalance
	if accountID == "" {
		b = s.PlatformBalance
	} else {
		if existing, ok := s.Balances[accountID]; ok {
			b = existing
		} else {
			b = NewAccountBalance()
			s.Balances[accountID] = b
		}
	}
	if b.Available[currency] < amount {
		return fmt.Errorf("insufficient funds: available %d, requested %d", b.Available[currency], amount)
	}
	b.Available[currency] -= amount
	return nil
}

// RecordBalanceTransaction creates and stores a balance transaction ledger entry.
func (s *MemoryStore) RecordBalanceTransaction(txType, source, currency string, amount, fee int64) string {
	id := s.BalanceTransactions.NextID()
	bt := BalanceTransaction{
		ID:       id,
		Object:   "balance_transaction",
		Amount:   amount,
		Currency: currency,
		Net:      amount - fee,
		Fee:      fee,
		Status:   "available",
		Type:     txType,
		Source:   source,
		Created:  s.Now(),
	}
	s.BalanceTransactions.Set(id, bt)
	return id
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Accounts              map[string]Account              `json:"accounts"`
	ExternalAccts         map[string]ExternalAccount      `json:"external_accounts"`
	Transfers             map[string]Transfer             `json:"transfers"`
	Payouts               map[string]Payout               `json:"payouts"`
	Events                map[string]Event                `json:"events"`
	BalanceTransactions   map[string]BalanceTransaction   `json:"balance_transactions"`
	Customers             map[string]Customer             `json:"customers"`
	Products              map[string]Product              `json:"products"`
	Prices                map[string]Price                `json:"prices"`
	PaymentIntents        map[string]PaymentIntent        `json:"payment_intents"`
	PaymentMethods        map[string]PaymentMethod        `json:"payment_methods"`
	Charges               map[string]Charge               `json:"charges"`
	Refunds               map[string]Refund               `json:"refunds"`
	Subscriptions         map[string]Subscription         `json:"subscriptions"`
	Invoices              map[string]Invoice              `json:"invoices"`
	InvoiceItems          map[string]InvoiceItem          `json:"invoice_items"`
	Coupons               map[string]Coupon               `json:"coupons"`
	SetupIntents          map[string]SetupIntent          `json:"setup_intents"`
	TaxRates              map[string]TaxRate              `json:"tax_rates"`
	Disputes              map[string]Dispute              `json:"disputes"`
	CheckoutSessions      map[string]CheckoutSession      `json:"checkout_sessions"`
	PaymentLinks          map[string]PaymentLink          `json:"payment_links"`
	Tokens                map[string]Token                `json:"tokens"`
	Sources               map[string]Source               `json:"sources"`
	Mandates              map[string]Mandate              `json:"mandates"`
	ConfirmationTokens    map[string]ConfirmationToken    `json:"confirmation_tokens"`
	CreditNotes           map[string]CreditNote           `json:"credit_notes"`
	PromotionCodes        map[string]PromotionCode        `json:"promotion_codes"`
	SubItems              map[string]SubscriptionItem     `json:"sub_items"`
	Quotes                map[string]Quote                `json:"quotes"`
	BillingPortalSessions map[string]BillingPortalSession `json:"billing_portal_sessions"`
	Reviews               map[string]Review               `json:"reviews"`
	TaxIDs                map[string]TaxID                `json:"tax_ids"`
	WebhookEndpoints      map[string]WebhookEndpoint      `json:"webhook_endpoints"`
	Files                 map[string]File                 `json:"files"`
	FileLinks             map[string]FileLink             `json:"file_links"`
	ShippingRates         map[string]ShippingRate         `json:"shipping_rates"`
	ApplicationFees       map[string]ApplicationFee       `json:"application_fees"`
	ApplicationFeeRefunds map[string]ApplicationFeeRefund `json:"application_fee_refunds"`
	TransferReversals     map[string]TransferReversal     `json:"transfer_reversals"`
	Persons               map[string]Person               `json:"persons"`
	TopUps                map[string]TopUp                `json:"topups"`
	Balances              map[string]*AccountBalance      `json:"balances"`
	PlatformBalance       *AccountBalance                 `json:"platform_balance"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Accounts:              s.Accounts.Snapshot(),
		ExternalAccts:         s.ExternalAccts.Snapshot(),
		Transfers:             s.Transfers.Snapshot(),
		Payouts:               s.Payouts.Snapshot(),
		Events:                s.Events.Snapshot(),
		BalanceTransactions:   s.BalanceTransactions.Snapshot(),
		Customers:             s.Customers.Snapshot(),
		Products:              s.Products.Snapshot(),
		Prices:                s.Prices.Snapshot(),
		PaymentIntents:        s.PaymentIntents.Snapshot(),
		PaymentMethods:        s.PaymentMethods.Snapshot(),
		Charges:               s.Charges.Snapshot(),
		Refunds:               s.Refunds.Snapshot(),
		Subscriptions:         s.Subscriptions.Snapshot(),
		Invoices:              s.Invoices.Snapshot(),
		InvoiceItems:          s.InvoiceItems.Snapshot(),
		Coupons:               s.Coupons.Snapshot(),
		SetupIntents:          s.SetupIntents.Snapshot(),
		TaxRates:              s.TaxRates.Snapshot(),
		Disputes:              s.Disputes.Snapshot(),
		CheckoutSessions:      s.CheckoutSessions.Snapshot(),
		PaymentLinks:          s.PaymentLinks.Snapshot(),
		Tokens:                s.Tokens.Snapshot(),
		Sources:               s.Sources.Snapshot(),
		Mandates:              s.Mandates.Snapshot(),
		ConfirmationTokens:    s.ConfirmationTokens.Snapshot(),
		CreditNotes:           s.CreditNotes.Snapshot(),
		PromotionCodes:        s.PromotionCodes.Snapshot(),
		SubItems:              s.SubItems.Snapshot(),
		Quotes:                s.Quotes.Snapshot(),
		BillingPortalSessions: s.BillingPortalSessions.Snapshot(),
		Reviews:               s.Reviews.Snapshot(),
		TaxIDs:                s.TaxIDs.Snapshot(),
		WebhookEndpoints:      s.WebhookEndpoints.Snapshot(),
		Files:                 s.Files.Snapshot(),
		FileLinks:             s.FileLinks.Snapshot(),
		ShippingRates:         s.ShippingRates.Snapshot(),
		ApplicationFees:       s.ApplicationFees.Snapshot(),
		ApplicationFeeRefunds: s.ApplicationFeeRefunds.Snapshot(),
		TransferReversals:     s.TransferReversals.Snapshot(),
		Persons:               s.Persons.Snapshot(),
		TopUps:                s.TopUps.Snapshot(),
		Balances:              s.snapshotBalances(),
		PlatformBalance:       s.PlatformBalance,
	}
}

func (s *MemoryStore) snapshotBalances() map[string]*AccountBalance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*AccountBalance, len(s.Balances))
	for k, v := range s.Balances {
		out[k] = v
	}
	return out
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	s.Accounts.LoadSnapshot(snap.Accounts)
	s.ExternalAccts.LoadSnapshot(snap.ExternalAccts)
	s.Transfers.LoadSnapshot(snap.Transfers)
	s.Payouts.LoadSnapshot(snap.Payouts)
	s.Events.LoadSnapshot(snap.Events)
	s.BalanceTransactions.LoadSnapshot(snap.BalanceTransactions)
	s.Customers.LoadSnapshot(snap.Customers)
	s.Products.LoadSnapshot(snap.Products)
	s.Prices.LoadSnapshot(snap.Prices)
	s.PaymentIntents.LoadSnapshot(snap.PaymentIntents)
	s.PaymentMethods.LoadSnapshot(snap.PaymentMethods)
	s.Charges.LoadSnapshot(snap.Charges)
	s.Refunds.LoadSnapshot(snap.Refunds)
	s.Subscriptions.LoadSnapshot(snap.Subscriptions)
	s.Invoices.LoadSnapshot(snap.Invoices)
	s.InvoiceItems.LoadSnapshot(snap.InvoiceItems)
	s.Coupons.LoadSnapshot(snap.Coupons)
	s.SetupIntents.LoadSnapshot(snap.SetupIntents)
	s.TaxRates.LoadSnapshot(snap.TaxRates)
	s.Disputes.LoadSnapshot(snap.Disputes)
	s.CheckoutSessions.LoadSnapshot(snap.CheckoutSessions)
	s.PaymentLinks.LoadSnapshot(snap.PaymentLinks)
	s.Tokens.LoadSnapshot(snap.Tokens)
	s.Sources.LoadSnapshot(snap.Sources)
	s.Mandates.LoadSnapshot(snap.Mandates)
	s.ConfirmationTokens.LoadSnapshot(snap.ConfirmationTokens)
	s.CreditNotes.LoadSnapshot(snap.CreditNotes)
	s.PromotionCodes.LoadSnapshot(snap.PromotionCodes)
	s.SubItems.LoadSnapshot(snap.SubItems)
	s.Quotes.LoadSnapshot(snap.Quotes)
	s.BillingPortalSessions.LoadSnapshot(snap.BillingPortalSessions)
	s.Reviews.LoadSnapshot(snap.Reviews)
	s.TaxIDs.LoadSnapshot(snap.TaxIDs)
	s.WebhookEndpoints.LoadSnapshot(snap.WebhookEndpoints)
	s.Files.LoadSnapshot(snap.Files)
	s.FileLinks.LoadSnapshot(snap.FileLinks)
	s.ShippingRates.LoadSnapshot(snap.ShippingRates)
	s.ApplicationFees.LoadSnapshot(snap.ApplicationFees)
	s.ApplicationFeeRefunds.LoadSnapshot(snap.ApplicationFeeRefunds)
	s.TransferReversals.LoadSnapshot(snap.TransferReversals)
	s.Persons.LoadSnapshot(snap.Persons)
	s.TopUps.LoadSnapshot(snap.TopUps)

	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.Balances != nil {
		s.Balances = snap.Balances
	}
	if snap.PlatformBalance != nil {
		s.PlatformBalance = snap.PlatformBalance
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Accounts.Reset()
	s.ExternalAccts.Reset()
	s.Transfers.Reset()
	s.Payouts.Reset()
	s.Events.Reset()
	s.BalanceTransactions.Reset()
	s.Customers.Reset()
	s.Products.Reset()
	s.Prices.Reset()
	s.PaymentIntents.Reset()
	s.PaymentMethods.Reset()
	s.Charges.Reset()
	s.Refunds.Reset()
	s.Subscriptions.Reset()
	s.Invoices.Reset()
	s.InvoiceItems.Reset()
	s.Coupons.Reset()
	s.SetupIntents.Reset()
	s.TaxRates.Reset()
	s.Disputes.Reset()
	s.CheckoutSessions.Reset()
	s.PaymentLinks.Reset()
	s.Tokens.Reset()
	s.Sources.Reset()
	s.Mandates.Reset()
	s.ConfirmationTokens.Reset()
	s.CreditNotes.Reset()
	s.PromotionCodes.Reset()
	s.SubItems.Reset()
	s.Quotes.Reset()
	s.BillingPortalSessions.Reset()
	s.Reviews.Reset()
	s.TaxIDs.Reset()
	s.WebhookEndpoints.Reset()
	s.Files.Reset()
	s.FileLinks.Reset()
	s.ShippingRates.Reset()
	s.ApplicationFees.Reset()
	s.ApplicationFeeRefunds.Reset()
	s.TransferReversals.Reset()
	s.Persons.Reset()
	s.TopUps.Reset()
	s.Clock.Reset()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Balances = make(map[string]*AccountBalance)
	s.PlatformBalance = NewAccountBalance()
}
