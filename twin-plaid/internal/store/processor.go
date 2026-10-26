package store

// ProcessorToken lets a partner read one account of an Item. A generic one
// (processor-sandbox-{uuid}) names its processor; a Stripe bank account token
// (btok_...) is made for the Stripe processor. Both end with their Item: Plaid
// revokes a processor's access only by removing the Item.
type ProcessorToken struct {
	Token     string `json:"processor_token"`
	ItemID    string `json:"item_id"`
	AccountID string `json:"account_id"`
	Processor string `json:"processor"`
}

// CreateProcessorToken issues a generic processor token for an account.
func (s *MemoryStore) CreateProcessorToken(itemID, accountID, processor string) ProcessorToken {
	t := ProcessorToken{Token: s.newToken("processor"), ItemID: itemID, AccountID: accountID, Processor: processor}
	s.ProcessorTokens.Set(t.Token, t)
	return t
}

// CreateStripeBankAccountToken issues a Stripe bank account token, btok_ and
// 24 base62 characters like Stripe's own, for an account.
func (s *MemoryStore) CreateStripeBankAccountToken(itemID, accountID string) ProcessorToken {
	b := make([]byte, 24)
	for i := range b {
		b[i] = base62[s.Rand.Intn(len(base62))]
	}
	t := ProcessorToken{Token: "btok_" + string(b), ItemID: itemID, AccountID: accountID, Processor: "stripe"}
	s.ProcessorTokens.Set(t.Token, t)
	return t
}

// removeProcessorTokens drops every processor token of an Item.
func (s *MemoryStore) removeProcessorTokens(itemID string) {
	for _, t := range s.ProcessorTokens.List() {
		if t.ItemID == itemID {
			s.ProcessorTokens.Delete(t.Token)
		}
	}
}
