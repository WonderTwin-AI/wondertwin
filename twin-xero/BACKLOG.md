# twin-xero Behavior Backlog

Work items to deepen twin-xero from "routes exist" to "behaviors work".

## High Value

### 1. Update and delete endpoints
- [ ] PUT /Contacts/{id} — update existing contacts
- [ ] PUT /Invoices/{id} — update draft invoices
- [ ] PUT /Items/{id} — update items
- [ ] PUT /Accounts/{id} — update accounts
- [ ] DELETE /Invoices/{id} — soft-delete (set status=DELETED)
- [ ] DELETE /Contacts/{id} — archive contact
- [ ] DELETE /Items/{id} — delete item

Currently can only create resources; no way to modify or remove them.

### 2. Invoice void/delete
- [ ] POST to transition invoice status to VOIDED
- [ ] Voiding reverses journal entries via engine
- [ ] DELETE sets status to DELETED
- [ ] Voided/deleted invoices excluded from reports

Status values VOIDED and DELETED are defined in the manifest but have no handler logic.

### 3. Bank transaction ledger integration
- [ ] Creating a bank transaction posts journal entries (double-entry)
- [ ] RECEIVE: debit BankAccount, credit line item accounts
- [ ] SPEND: debit line item accounts, credit BankAccount
- [ ] Bank account balance maintained via engine

Currently bank transactions are stored but create no journal entries.

### 4. Credit note application to invoices
- [ ] Endpoint to allocate credit note against an invoice
- [ ] Reduces invoice AmountDue
- [ ] Decrements credit note RemainingCredit
- [ ] Updates invoice status to PAID if fully covered
- [ ] Posts offsetting journal entry

Credit notes exist but cannot be applied to invoices.

### 5. Contact validation on transactions
- [ ] Creating an invoice validates ContactID exists
- [ ] Creating a payment validates invoice ContactID
- [ ] Creating a bank transaction validates ContactID if provided

Currently accepts any ContactID string without checking.

## Medium Value

### 6. Partial payment support
- [ ] Multiple payments against a single invoice
- [ ] AmountDue tracks cumulative payments
- [ ] Status remains AUTHORISED until fully paid
- [ ] Test: two partial payments → PAID on second

### 7. Payment deletion/reversal
- [ ] DELETE /Payments/{id} — reverse a payment
- [ ] Re-credits invoice AmountDue
- [ ] Reverts invoice status from PAID to AUTHORISED if needed
- [ ] Reverses journal entries

### 8. Item reference in line items
- [ ] Line items reference ItemCode
- [ ] UnitAmount/AccountCode defaults from Item.SalesDetails or Item.PurchaseDetails
- [ ] Missing item returns error

Items are stored but never referenced when creating invoice line items.

### 9. Contact balance tracking
- [ ] Contact.Balances updated when invoices created/paid
- [ ] AccountsReceivable/AccountsPayable totals maintained

## Lower Priority

### 10. Tax rate support
- [ ] Tax rates stored and retrievable
- [ ] TaxType on line items resolved to tax rate
- [ ] TaxAmount calculated per line and on invoice total

### 11. Tracking categories
- [ ] CRUD for tracking categories and options
- [ ] TrackingCategoryID on line items for departmental reporting

### 12. Multi-currency basics
- [ ] CurrencyCode on invoices used in journal entries
- [ ] CurrencyRate field applied to convert to base currency

### 13. Prepayments and overpayments
- [ ] Prepayment entity type (payment before invoice)
- [ ] Overpayment entity type (excess payment)
- [ ] Allocation to invoices
