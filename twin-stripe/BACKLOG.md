# twin-stripe Behavior Backlog

Work items to deepen twin-stripe from "routes exist" to "behaviors work".

## High Value

### 1. ~~Update handlers for core payment resources~~ (done)
- [x] `POST /v1/charges/{id}` — update metadata, description
- [x] `POST /v1/invoices/{id}` — update metadata, description, collection_method (draft only)
- [x] `POST /v1/payment_intents/{id}` — update metadata, description, amount, currency, customer, payment_method

### 2. ~~Customer existence validation~~ (done)
- [x] `AttachPaymentMethod` — validate customer exists before attaching
- [x] `CreateSubscription` — validate customer exists before creating

### 3. ~~Customer deletion cascade~~ (done)
- [x] Cancel active subscriptions on customer delete
- [x] Detach payment methods on customer delete

## Medium Value

### 4. ~~Proration on subscription changes~~ (done)
- [x] Updating a subscription's price or quantity mid-cycle generates a prorated invoice
- [x] Credit for unused time on old price, charge for remaining time on new price
- [x] `proration_behavior=none` skips proration invoice

### 5. Invoice lifecycle deepening
- [ ] `POST /v1/invoices/{id}/send` for send_invoice mode
- [ ] Paying an invoice creates a PaymentIntent + Charge (currently just flips status)
- [ ] Finalizing an invoice sets `hosted_invoice_url`

### 6. Payout cancellation
- [ ] `POST /v1/payouts/{id}/cancel` for payouts in pending status
- [ ] Validate payout is in cancellable state
- [ ] Re-credit balance on cancellation

Currently only admin-fail exists for payouts.

## Lower Priority

### 7. Product/Price archival cascade
- [ ] Deleting a product deactivates its associated prices
- [ ] Deactivating a price prevents new subscription creation with it

### 8. Dispute lifecycle
- [ ] `POST /v1/disputes/{id}` to submit evidence
- [ ] `POST /v1/disputes/{id}/close` state transitions (needs_response → under_review → won/lost)
- [ ] Dispute creation debits balance, won dispute re-credits

### 9. 3D Secure / requires_action simulation
- [ ] Specific test card numbers trigger `requires_action` status on payment intents
- [ ] `POST /admin/payment_intents/{id}/authenticate` to simulate user completing 3DS
- [ ] Timeout → automatic cancellation after configurable delay
