# Fintech Twin Planning Skill

> **Role**: Planning agent. You produce a twin specification. You do not write implementation code.  
> **Inputs**: Research artifacts from a completed base research skill run on a fintech platform.  
> **Output**: A single twin spec document that fully defines what the coding agent will build.  
> **Depends on**: `wondertwin-docs/vertical-architecture/fintech/fintech-twin-architecture.md` — read it in full before executing this skill.  
> **Does not replace**: The base twin research skill. Research must be complete before planning begins.

---

## 0. Before You Start

Confirm both inputs are present:

1. **Research artifacts** from the base twin research skill run on this platform. At minimum: system model (sections 1.1–1.8 answered), compatibility path assessment, event model specification. Planning cannot begin without these — return to research if they are missing.

2. **`wondertwin-docs/vertical-architecture/fintech/fintech-twin-architecture.md`** — read it completely. This skill references it throughout and assumes you have internalized the seven primitives, the hook architecture, and the `twinkit/` package map. Do not proceed if you have not read it.

Your output is `twin-{platform}-spec.md`. Nothing else. The coding agent that runs after you reads this document and builds from it. If something is not in the spec, the coding agent will not build it.

---

## 1. Primitive Classification

This is the first section of the spec. Derive it from the research artifacts. Do not guess — every field must trace to a finding in the research.

Produce the classification JSON defined in the architecture document (Section 1, Classification Output). Then write a narrative for each primitive present that explains:

- Why this primitive is present (what the platform does that requires it)
- Which specific variant applies (banking vs. accounting for ledger; JIT vs. pre-funded for card; direct vs. middleware for movement)
- What the key behavioral constraint is — the one thing about this platform's implementation of this primitive that differs from the baseline and will require a hook

If a primitive is absent, one sentence confirming it is absent and why is sufficient. Do not write about absent primitives at length.

**Classification validation.** Before moving to Section 2, answer these three questions:

1. Does the classification match the research findings exactly? If research says the platform routes through partner banks, the classification must say `platform_authority: middleware`, not `direct`.
2. Is the JIT vs. pre-funded decision confirmed from documentation, not inferred? If it is inferred, flag it as `confidence: low` and identify what additional research would confirm it.
3. Are there any primitives where you are uncertain whether they are present? Mark them `present: uncertain` and explain what would resolve the uncertainty. Do not silently omit uncertain primitives.

---

## 2. `twinkit/` Import Plan

List the packages this twin will import from `twinkit/`. Use the package map in the architecture document (Section 5). For each package:

- State whether it exists (built) or needs to be built.
- If it needs to be built, flag it explicitly — this twin may be the first to use this package, and the coding agent needs to know to build the package before the twin.
- If the package has sub-packages and only some are needed, specify which.

Format:

```
twinkit/ledger/journal          EXISTS — required
twinkit/ledger/accounting       EXISTS — required
twinkit/movement                NEEDS BUILD — this is the first movement twin
twinkit/movement/ach            NEEDS BUILD
twinkit/movement/book           NEEDS BUILD
```

If a primitive is present but the `twinkit/` package does not exist and this twin is not intended to be the first twin to build it, flag this as a dependency gap that must be resolved before this twin can be built. The planning agent does not decide which twin builds a package first — it flags the gap.

---

## 3. Hook Specifications

For each primitive present, specify every hook that requires a non-trivial implementation for this platform. "Non-trivial" means: not a no-op, and not a generic pattern that applies to all platforms using this primitive.

For each hook, write:

**Hook name**: `OnAuthorizationRequested`  
**Primitive**: Card  
**Reason required**: This platform uses JIT funding. The hook must make an outbound HTTP POST to the configured JIT webhook URL, wait for a response within the configured timeout, and return the funding decision. If the webhook is unreachable or times out, return decline.  
**Inputs**: `AuthorizationRequest` — contains card ID, amount, currency, MCC, merchant name, merchant ID  
**Output**: `AuthorizationDecision` — approved or declined, with reason code if declined  
**Timeout behavior**: Decline with reason `jit_timeout` if no response within `cfg.JITTimeoutMS`  
**Error behavior**: Decline with reason `jit_unreachable` if the webhook endpoint returns non-2xx or connection fails  
**Platform quirk**: [Document any platform-specific field in the request or response that differs from the standard hook signature — e.g., "This platform sends the merchant's legal name in addition to the display name; include both in the outbound payload."]  

Write a hook specification for every hook that is non-trivial. If a hook is a no-op for this platform (e.g., `OnDisputeInitiated` for a platform that has no dispute surface), state that it is a no-op and will use the default. Do not leave hooks unaddressed.

**Mandatory hooks** — these are never no-ops. Write a full specification for each that is applicable to the primitive composition:

| Primitive | Mandatory hook | Why never a no-op |
|---|---|---|
| Money Movement | `OnPaymentCreated` | Must fire webhook event for every payment; webhook schema is platform-specific |
| Money Movement | `OnReturnReceived` | Return code translation from engine codes to platform-specific codes is always required |
| Banking Ledger | `OnBalanceQuery` | Balance field names and relationships are platform-specific |
| Accounting Ledger | `OnDocumentStateTransition` | Document state names and webhook event names are platform-specific |
| Card | `OnAuthorizationRequested` | Pre-funded or JIT — both require platform-specific implementation |
| Card | `OnAuthorizationCompleted` | Must fire authorization webhook event; schema is platform-specific |
| Card | `OnClearingReceived` | Clearing event webhook schema is platform-specific |

---

## 4. State Model

Describe every entity the twin stores in memory and its complete field set. Derive this from the research artifacts (section 1.3: What does this system remember?).

For each entity:

**Entity name**: `Payment`  
**Primitive**: Money Movement  
**Fields**:
- `id` string — platform-assigned ID, format `{prefix}_{random}` e.g. `payment_abc123`
- `amount` int64 — in minor currency units (cents for USD)
- `currency` string — ISO 4217, three characters
- `source_account_id` string — references Account entity
- `destination_account_id` string — references Account entity
- `rail` enum — `ACH | RTP | wire | book`
- `status` enum — `pending | processing | settled | failed | returned | reversed`
- `idempotency_key` string — optional, enforced unique if present
- `created_at` time.Time
- `settled_at` *time.Time — nil until settled
- [additional platform-specific fields]

**State machine**: `pending → processing → settled | failed`. From `settled`: `→ returned` (ACH only, within return window). From any terminal state: no further transitions.

**Platform-specific fields** — fields that exist on this platform's API but have no equivalent in the engine model:
- `network` string — Increase-specific: which Fed network processed the payment (`FedACH | FedWire | FedNow`)
- `transaction_id` string — Increase-specific: ID of the associated transaction on the account ledger

Write this section for every entity the twin exposes via its API. Group by primitive. If an entity belongs to two primitives (e.g., a payment that also creates a ledger transaction), assign it to the primary primitive and note the cross-primitive effect.

---

## 5. API Surface

List every endpoint the twin will expose. Derive this from the research artifacts (section 1.2: How does this system expose itself? and section 1.3: What does this system remember?).

Format:

```
POST   /v1/payments                    Create payment — movement engine
GET    /v1/payments                    List payments — movement engine, pagination
GET    /v1/payments/{id}               Get payment — movement engine
POST   /v1/payments/{id}/cancel        Cancel payment — movement engine, OnReversalRequested hook
GET    /v1/accounts                    List accounts — ledger engine
GET    /v1/accounts/{id}               Get account with balances — ledger engine, OnBalanceQuery hook
GET    /v1/accounts/{id}/transactions  List account transactions — ledger engine
POST   /v1/webhooks                    Register webhook endpoint
GET    /v1/webhooks                    List registered webhooks
DELETE /v1/webhooks/{id}               Delete webhook endpoint
```

For each endpoint, note:
- Which engine handles it
- Which hooks are called during its execution
- Any endpoint-specific behavioral constraints not captured by the engine invariants

**Pagination**: For every list endpoint, specify the pagination model: cursor-based, page-based, or offset-based. The model is platform-specific. Document the request parameters (e.g., `after`, `before`, `limit` for cursor-based; `page`, `per_page` for page-based) and the response envelope (where is the data, where is the cursor/next page indicator).

**Error format**: Specify the platform's error response shape. This is a mandatory specification — it determines how the twin's handlers translate engine errors into HTTP responses. Example:

```json
{
  "status": 422,
  "error": {
    "code": "insufficient_funds",
    "message": "The source account has insufficient available balance.",
    "detail": {
      "available_balance": 45000,
      "requested_amount": 100000
    }
  }
}
```

Map engine error types to platform error codes. If the engine returns `ErrInsufficientBalance`, the handler for this platform returns `HTTP 422` with `code: "insufficient_funds"`. Write this mapping for every engine error type the twin's endpoints can produce.

---

## 6. Webhook Event Catalog

List every webhook event the twin fires. For each event:

**Event name**: `payment.settled`  
**Primitive**: Money Movement  
**Trigger**: Payment transitions from `processing` to `settled`  
**Delivery**: Via `twinkit/webhook` — enqueued in `OnPaymentCreated` hook for creation events; enqueued in `OnSettlementAttempt` hook for settlement events  
**Payload shape**:
```json
{
  "id": "event_abc123",
  "type": "payment.settled",
  "created_at": "2026-03-16T12:00:00Z",
  "data": {
    "object": {
      "id": "payment_xyz789",
      "amount": 50000,
      "currency": "USD",
      "status": "settled",
      "settled_at": "2026-03-16T12:00:00Z"
    }
  }
}
```
**Signing**: HMAC-SHA256 via `twinkit/webhook` — specify the header name used (e.g., `Increase-Webhook-Signature`, `Stripe-Signature`, `X-Webhook-Signature`)

**Delivery semantics**: At-least-once (standard for all WonderTwin webhooks via `twinkit/webhook`). Note any platform-specific retry behavior (e.g., "This platform retries up to 5 times with exponential backoff starting at 1 minute").

---

## 7. Seed Data Specification

Define the complete seed data schema for this twin. Produce two things:

**7.1 Annotated seed schema** — the JSON schema for this twin's seed file, with comments explaining each field and what it controls. Reference the behavior configuration format from the architecture document (Section 6) for the `behavior` section. The `state` section is twin-specific.

**7.2 Reference seed file** — a complete, valid seed file that represents a realistic starting state for integration testing. This seed file should:
- Create at least two accounts in usable states (active, funded or with configured behaviors)
- Configure at least one behavioral rule for each primitive present (e.g., one ACH return rule, one card decline rule, one identity rejection scenario)
- Include at least one pre-existing entity that tests can reference without creating it first (e.g., an existing customer with a known ID that scenario tests can use)

The reference seed file becomes the default `{platform}-seed.json` shipped with the twin. It is documentation as much as configuration.

---

## 8. Conformance Test Plan

Specify the conformance tests this twin must pass before it can be published to the registry. Two types:

**8.1 Engine invariant tests** — inherited from `twinkit/`. These run against the interface defined in each primitive's `interface.go`. List which conformance suites apply based on the primitive composition:

- `twinkit/movement/conformance` — if money movement is present
- `twinkit/ledger/banking/conformance` — if banking ledger is present
- `twinkit/ledger/accounting/conformance` — if accounting ledger is present
- `twinkit/card/conformance` — if card is present
- `twinkit/identity/conformance` — if identity is present
- `twinkit/risk/conformance` — if risk is present
- `twinkit/billing/conformance` — if billing is present
- `twinkit/compliance/conformance` — if compliance is present

These tests are not written by the planning or implementation agent — they exist in the `twinkit/` packages. The twin only needs to pass them.

**8.2 Platform-specific behavioral tests** — tests that verify the platform's unique behavior, beyond what the engine invariants cover. Write these as scenario descriptions; the coding agent implements them as Go tests in `handlers_test.go`.

For each platform-specific behavioral test:

**Test**: ACH return code translation  
**Scenario**: Create an ACH payment to the account number configured to trigger R01 return. Advance time past the settlement delay. Verify a return event fires with the platform's representation of R01.  
**Why not an engine test**: The engine tests that a return is generated. This test verifies that the platform-specific return code name (`"insufficient_funds"` on this platform, not `"R01"`) appears in the return event and API response.

Write a behavioral test specification for every platform-specific behavioral requirement identified in the research artifacts and hook specifications. At minimum, write tests for:
- Every return/error code that appears in the platform's documentation with a specific user-visible value
- Every platform-specific field that has non-obvious behavior (computed from other fields, defaulted non-obviously, conditionally required)
- Every multi-step workflow that spans more than one API call (e.g., KYC flow, card onboarding, ACH micro-deposit verification)

---

## 9. Out-of-Scope Register

Explicitly list what this twin does NOT implement in v0.1, and why. This is required. An implementation scope that is not explicitly bounded will expand without limit.

Format:

| Surface | Reason out of scope | Phase to add |
|---|---|---|
| UK payroll API | Regulatory complexity, separate API surface, separate research required | Phase 3 |
| Dispute representment | Low integration demand, complex multi-step workflow | Phase 2 |
| Multi-currency FX conversion | FX rate data source not resolved; seed-configurable rates deferred | Phase 2 |
| SWIFT international wires | Wire twin covers domestic Fedwire only | Phase 3 |

Every item in the out-of-scope register becomes a gap in the feature gap register (existing WonderTwin standard artifact). They are linked — the feature gap register is the customer-facing version; the out-of-scope register is the planning-time version.

---

## 10. Build Sequencing Notes

If this twin depends on `twinkit/` packages that do not yet exist, write build sequencing notes for the coding agent:

**Build this first**: List the `twinkit/` packages that must be built before the twin. Include the interface they must satisfy (file path in `twinkit/`).

**Build order within the twin**: If there are dependencies within the twin itself (e.g., the ledger engine must be initialized before the movement engine because the movement engine creates ledger entries), document them.

**Validation checkpoints**: Define the checkpoints where the coding agent should stop and verify before continuing:
1. After building `twinkit/movement`: run `twinkit/movement/conformance` suite against a stub twin.
2. After implementing `OnAuthorizationRequested` hook: run the hook unit tests in isolation before wiring to the router.
3. After completing the full twin: run all conformance suites and platform-specific behavioral tests.

---

## Spec Completeness Check

Before delivering the spec, verify:

- [ ] Every primitive in the classification has at least one hook specification (or explicit no-op justification)
- [ ] Every API endpoint is listed in Section 5
- [ ] Every webhook event in the research artifacts appears in Section 6
- [ ] The reference seed file in Section 7.2 is valid JSON and covers every primitive present
- [ ] Every item in the out-of-scope register has a phase assignment
- [ ] No `twinkit/` package is listed as `EXISTS` if it has not been built yet
- [ ] If any classification field has `confidence: low`, the spec notes what research would resolve it and marks the affected hook or behavior as provisional

If any item above is not satisfied, the spec is incomplete. Do not deliver an incomplete spec. Return to the relevant section and complete it. An incomplete spec delivered to the coding agent produces an incomplete twin.

---

*This skill produces one artifact: `twin-{platform}-spec.md`. Deliver that document. Do not deliver analysis, summaries, or intermediate findings — only the spec.*
