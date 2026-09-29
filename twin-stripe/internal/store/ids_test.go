package store

import (
	"regexp"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
)

func TestStripeID_OpaqueAndSeeded(t *testing.T) {
	a, b := New(), New()
	a.Rand, b.Rand = sim.NewRand(7), sim.NewRand(7)

	cus := a.StripeID(a.Customers.NextID())
	if !regexp.MustCompile(`^cus_[0-9A-Za-z]{14}$`).MatchString(cus) {
		t.Fatalf("expected a 14-character customer ID, got %s", cus)
	}
	if got := b.StripeID(b.Customers.NextID()); got != cus {
		t.Fatalf("expected the same seed to reproduce the ID, got %s and %s", cus, got)
	}
	pi := a.StripeID(a.PaymentIntents.NextID())
	if !regexp.MustCompile(`^pi_[0-9A-Za-z]{24}$`).MatchString(pi) {
		t.Fatalf("expected a 24-character payment intent ID, got %s", pi)
	}
	if cs := a.StripeID(a.CheckoutSessions.NextID()); !regexp.MustCompile(`^cs_test_[0-9A-Za-z]{24}$`).MatchString(cs) {
		t.Fatalf("expected a cs_test_ checkout session ID, got %s", cs)
	}
}
