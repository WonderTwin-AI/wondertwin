package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// -- helpers --------------------------------------------------------------

func newTestEngine(t *testing.T) (*Engine, *MemoryStore, *state.Clock) {
	t.Helper()
	clk := state.NewClock()
	clk.Pin(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
	store := NewMemoryStore()
	eng := NewEngine(WithStore(store), WithClock(clk))
	return eng, store, clk
}

func ptr(s string) *string { return &s }

func twoSignerInput() CreateInput {
	return CreateInput{
		Parties: []Party{
			{ID: "p1", Role: RoleSigner, RoutingOrder: 0, Required: true, Contact: "alice@example.com"},
			{ID: "p2", Role: RoleSigner, RoutingOrder: 1, Required: true, Contact: "bob@example.com"},
		},
	}
}

// -- create / update ------------------------------------------------------

func TestCreate_AssignsIDAndAuditEntry(t *testing.T) {
	eng, _, clk := newTestEngine(t)
	c, err := eng.Create(context.Background(), twoSignerInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID != "ctr_000001" {
		t.Errorf("ID = %q, want ctr_000001", c.ID)
	}
	if c.Status != StatusDraft {
		t.Errorf("status = %s, want DRAFT", c.Status)
	}
	if got, want := c.CreatedAt, clk.Now(); !got.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", got, want)
	}
	if len(c.AuditLog) != 1 {
		t.Fatalf("audit log len = %d, want 1", len(c.AuditLog))
	}
	if c.AuditLog[0].EventType != AuditCreated {
		t.Errorf("audit[0].EventType = %s, want %s", c.AuditLog[0].EventType, AuditCreated)
	}
	if c.AuditLog[0].Sequence != 1 {
		t.Errorf("audit[0].Sequence = %d, want 1", c.AuditLog[0].Sequence)
	}
}

func TestCreate_RespectsExplicitID(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	in := twoSignerInput()
	in.ID = "custom-id"
	c, err := eng.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID != "custom-id" {
		t.Errorf("ID = %q, want custom-id", c.ID)
	}
}

func TestCreate_RejectsNoSigner(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	_, err := eng.Create(context.Background(), CreateInput{
		Parties: []Party{{ID: "p1", Role: RoleCC, RoutingOrder: 0}},
	})
	if !errors.Is(err, ErrInvalidParty) {
		t.Fatalf("err = %v, want ErrInvalidParty", err)
	}
}

func TestCreate_RejectsDuplicatePartyID(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	_, err := eng.Create(context.Background(), CreateInput{
		Parties: []Party{
			{ID: "p1", Role: RoleSigner, Required: true},
			{ID: "p1", Role: RoleSigner, Required: true},
		},
	})
	if !errors.Is(err, ErrInvalidParty) {
		t.Fatalf("err = %v, want ErrInvalidParty", err)
	}
}

func TestUpdate_OnlyDraft(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	if _, err := eng.Send(context.Background(), c.ID); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, err := eng.Update(context.Background(), c.ID, Mutation{})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("Update on SENT err = %v, want ErrInvalidStatus", err)
	}
}

// -- send / signature / execute --------------------------------------------

func TestHappyPath_SequentialSigners(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, err := eng.Create(context.Background(), twoSignerInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := eng.Send(context.Background(), c.ID); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// p2 cannot sign first (routing-order 1 not yet active)
	_, err = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{AuthMethod: "email_link"})
	if !errors.Is(err, ErrPartyNotInActiveSet) {
		t.Fatalf("p2 first-sign err = %v, want ErrPartyNotInActiveSet", err)
	}

	// p1 signs → PARTIALLY_SIGNED
	c, err = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{AuthMethod: "email_link"})
	if err != nil {
		t.Fatalf("p1 sign: %v", err)
	}
	if c.Status != StatusPartiallySigned {
		t.Errorf("after p1 status = %s, want PARTIALLY_SIGNED", c.Status)
	}

	// p2 signs → SIGNED → EXECUTED
	c, err = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{AuthMethod: "sso"})
	if err != nil {
		t.Fatalf("p2 sign: %v", err)
	}
	if c.Status != StatusExecuted {
		t.Errorf("final status = %s, want EXECUTED", c.Status)
	}
	if c.ExecutedAt.IsZero() {
		t.Error("ExecutedAt is zero")
	}
}

func TestParallelSigners_BothMustSign(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), CreateInput{
		Parties: []Party{
			{ID: "p1", Role: RoleSigner, RoutingOrder: 0, Required: true},
			{ID: "p2", Role: RoleSigner, RoutingOrder: 0, Required: true},
		},
	})
	_, _ = eng.Send(context.Background(), c.ID)

	// Either order is fine — both at order 0
	if _, err := eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{}); err != nil {
		t.Fatalf("p2 sign: %v", err)
	}
	c, err := eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	if err != nil {
		t.Fatalf("p1 sign: %v", err)
	}
	if c.Status != StatusExecuted {
		t.Errorf("status = %s, want EXECUTED", c.Status)
	}
}

func TestRecordSignature_AlreadySigned(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	_, _ = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	_, err := eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	if !errors.Is(err, ErrAlreadySigned) {
		t.Fatalf("err = %v, want ErrAlreadySigned", err)
	}
}

func TestRecordSignature_CCCannotSign(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), CreateInput{
		Parties: []Party{
			{ID: "p1", Role: RoleSigner, Required: true},
			{ID: "cc1", Role: RoleCC},
		},
	})
	_, _ = eng.Send(context.Background(), c.ID)
	_, err := eng.RecordSignature(context.Background(), c.ID, "cc1", Attestation{})
	if !errors.Is(err, ErrInvalidParty) {
		t.Fatalf("err = %v, want ErrInvalidParty", err)
	}
}

// -- witness pairing ------------------------------------------------------

func TestWitness_PairedRequiredToExecute(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, err := eng.Create(context.Background(), CreateInput{
		Parties: []Party{
			{ID: "signer", Role: RoleSigner, RoutingOrder: 0, Required: true},
			{ID: "witness", Role: RoleWitness, RoutingOrder: 0, PairedWith: ptr("signer")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, _ = eng.Send(context.Background(), c.ID)

	// Signing only the signer is not enough.
	c, err = eng.RecordSignature(context.Background(), c.ID, "signer", Attestation{})
	if err != nil {
		t.Fatalf("signer sign: %v", err)
	}
	if c.Status != StatusPartiallySigned {
		t.Errorf("after signer status = %s, want PARTIALLY_SIGNED", c.Status)
	}

	// Witness signs in same routing slot → EXECUTED.
	c, err = eng.RecordSignature(context.Background(), c.ID, "witness", Attestation{})
	if err != nil {
		t.Fatalf("witness sign: %v", err)
	}
	if c.Status != StatusExecuted {
		t.Errorf("after witness status = %s, want EXECUTED", c.Status)
	}
}

func TestWitness_PairingValidation(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	tests := []struct {
		name    string
		parties []Party
	}{
		{
			name: "paired with unknown party",
			parties: []Party{
				{ID: "s", Role: RoleSigner, Required: true},
				{ID: "w", Role: RoleWitness, PairedWith: ptr("nope")},
			},
		},
		{
			name: "paired with non-signer",
			parties: []Party{
				{ID: "s", Role: RoleSigner, Required: true},
				{ID: "cc", Role: RoleCC},
				{ID: "w", Role: RoleWitness, PairedWith: ptr("cc")},
			},
		},
		{
			name: "routing-order mismatch",
			parties: []Party{
				{ID: "s", Role: RoleSigner, RoutingOrder: 0, Required: true},
				{ID: "w", Role: RoleWitness, RoutingOrder: 1, PairedWith: ptr("s")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eng.Create(context.Background(), CreateInput{Parties: tt.parties})
			if !errors.Is(err, ErrWitnessPairing) {
				t.Fatalf("err = %v, want ErrWitnessPairing", err)
			}
		})
	}
}

// -- decline / void / expire ----------------------------------------------

func TestDecline(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	c, err := eng.Decline(context.Background(), c.ID, "p1", "no thanks")
	if err != nil {
		t.Fatalf("Decline: %v", err)
	}
	if c.Status != StatusDeclined {
		t.Errorf("status = %s, want DECLINED", c.Status)
	}
	// Cannot record signature on DECLINED.
	_, err = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("sign on DECLINED err = %v, want ErrInvalidStatus", err)
	}
}

func TestVoid(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	c, err := eng.Void(context.Background(), c.ID, "rescinded")
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if c.Status != StatusVoided {
		t.Errorf("status = %s, want VOIDED", c.Status)
	}
	_, err = eng.Void(context.Background(), c.ID, "again")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("re-void err = %v, want ErrInvalidStatus", err)
	}
}

func TestExpire(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	c, err := eng.Expire(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if c.Status != StatusExpired {
		t.Errorf("status = %s, want EXPIRED", c.Status)
	}
}

// -- approval chain reserved ----------------------------------------------

func TestApprovalChain_NotImplementedV01(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, err := eng.SubmitForApproval(context.Background(), c.ID)
	if !errors.Is(err, ErrApprovalChainNotImplemented) {
		t.Errorf("SubmitForApproval err = %v, want ErrApprovalChainNotImplemented", err)
	}
	_, err = eng.Approve(context.Background(), c.ID, "approver", nil, "")
	if !errors.Is(err, ErrApprovalChainNotImplemented) {
		t.Errorf("Approve err = %v, want ErrApprovalChainNotImplemented", err)
	}
}

// -- hooks ----------------------------------------------------------------

type recordingHooks struct {
	NoOpHooks
	executed       int
	sigRecorded    int
	transitions    []string // "FROM->TO"
	executeErr     error
	validateErr    error
}

func (r *recordingHooks) ValidateCreate(_ context.Context, _ *Contract) error {
	return r.validateErr
}
func (r *recordingHooks) OnExecuted(_ context.Context, _ *Contract) error {
	r.executed++
	return r.executeErr
}
func (r *recordingHooks) OnSignatureRecorded(_ context.Context, _ *Contract, _ *Party) error {
	r.sigRecorded++
	return nil
}
func (r *recordingHooks) OnStateTransition(_ context.Context, _ *Contract, from, to Status) error {
	r.transitions = append(r.transitions, string(from)+"->"+string(to))
	return nil
}

func TestHooks_OnExecutedFires(t *testing.T) {
	clk := state.NewClock()
	clk.Pin(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
	h := &recordingHooks{}
	eng := NewEngine(WithStore(NewMemoryStore()), WithClock(clk), WithHooks(h))
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	_, _ = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	_, _ = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{})
	if h.executed != 1 {
		t.Errorf("OnExecuted called %d times, want 1", h.executed)
	}
	if h.sigRecorded != 2 {
		t.Errorf("OnSignatureRecorded called %d times, want 2", h.sigRecorded)
	}
	wantTransitions := []string{"DRAFT->SENT", "SENT->PARTIALLY_SIGNED", "PARTIALLY_SIGNED->SIGNED", "SIGNED->EXECUTED"}
	if got := h.transitions; !equalStrings(got, wantTransitions) {
		t.Errorf("transitions = %v, want %v", got, wantTransitions)
	}
}

func TestHooks_OnExecutedErrorHaltsAtSigned(t *testing.T) {
	clk := state.NewClock()
	clk.Pin(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
	h := &recordingHooks{executeErr: errors.New("downstream failed")}
	store := NewMemoryStore()
	eng := NewEngine(WithStore(store), WithClock(clk), WithHooks(h))
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	_, _ = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	_, err := eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{})
	if err == nil {
		t.Fatal("expected error from OnExecuted")
	}
	got, _ := store.Load(c.ID)
	if got.Status != StatusSigned {
		t.Errorf("status after hook error = %s, want SIGNED", got.Status)
	}
	// Manual recovery: clear the hook error and call Execute.
	h.executeErr = nil
	got2, err := eng.Execute(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("recovery Execute: %v", err)
	}
	if got2.Status != StatusExecuted {
		t.Errorf("status after recovery = %s, want EXECUTED", got2.Status)
	}
}

func TestHooks_ValidateCreateBlocksPersist(t *testing.T) {
	clk := state.NewClock()
	clk.Pin(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
	h := &recordingHooks{validateErr: errors.New("nope")}
	store := NewMemoryStore()
	eng := NewEngine(WithStore(store), WithClock(clk), WithHooks(h))
	_, err := eng.Create(context.Background(), twoSignerInput())
	if err == nil {
		t.Fatal("expected validate error")
	}
	all, _ := store.List()
	if len(all) != 0 {
		t.Errorf("store has %d contracts, want 0 (validate should block persist)", len(all))
	}
}

// -- audit log ------------------------------------------------------------

func TestAuditLog_MonotonicSequence(t *testing.T) {
	eng, _, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	_, _ = eng.Send(context.Background(), c.ID)
	_, _ = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{})
	c, _ = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{})
	for i, e := range c.AuditLog {
		if e.Sequence != uint64(i+1) {
			t.Errorf("audit[%d].Sequence = %d, want %d", i, e.Sequence, i+1)
		}
	}
	// Spot-check: last entry should be EXECUTED.
	last := c.AuditLog[len(c.AuditLog)-1]
	if last.EventType != AuditExecuted {
		t.Errorf("last audit event = %s, want %s", last.EventType, AuditExecuted)
	}
}

// -- determinism ----------------------------------------------------------

func TestDeterminism_SameInputsSameOutputs(t *testing.T) {
	run := func() *Contract {
		clk := state.NewClock()
		clk.Pin(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
		eng := NewEngine(WithStore(NewMemoryStore()), WithClock(clk))
		c, _ := eng.Create(context.Background(), twoSignerInput())
		_, _ = eng.Send(context.Background(), c.ID)
		_, _ = eng.RecordSignature(context.Background(), c.ID, "p1", Attestation{AuthMethod: "email_link"})
		c, _ = eng.RecordSignature(context.Background(), c.ID, "p2", Attestation{AuthMethod: "sso"})
		return c
	}
	a, b := run(), run()
	if a.ID != b.ID {
		t.Errorf("ID differs: %q vs %q", a.ID, b.ID)
	}
	if !a.ExecutedAt.Equal(b.ExecutedAt) {
		t.Errorf("ExecutedAt differs: %v vs %v", a.ExecutedAt, b.ExecutedAt)
	}
	if len(a.AuditLog) != len(b.AuditLog) {
		t.Fatalf("audit log lengths differ: %d vs %d", len(a.AuditLog), len(b.AuditLog))
	}
	for i := range a.AuditLog {
		if a.AuditLog[i].EventType != b.AuditLog[i].EventType {
			t.Errorf("audit[%d].EventType differs: %s vs %s", i, a.AuditLog[i].EventType, b.AuditLog[i].EventType)
		}
		if !a.AuditLog[i].At.Equal(b.AuditLog[i].At) {
			t.Errorf("audit[%d].At differs: %v vs %v", i, a.AuditLog[i].At, b.AuditLog[i].At)
		}
	}
}

// -- state-store isolation ------------------------------------------------

func TestStore_LoadReturnsCopy(t *testing.T) {
	eng, store, _ := newTestEngine(t)
	c, _ := eng.Create(context.Background(), twoSignerInput())
	got, _ := store.Load(c.ID)
	got.Status = StatusVoided // mutate copy
	got2, _ := store.Load(c.ID)
	if got2.Status != StatusDraft {
		t.Errorf("store leaked mutation: status = %s, want DRAFT", got2.Status)
	}
}

// -- util -----------------------------------------------------------------

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
