# twin-xero Behavior Backlog

Work items to deepen twin-xero from "routes exist" to "behaviors work".

## High Value

### 1. Update and delete endpoints
- [x] PUT /Contacts/{id} — update existing contacts
- [x] PUT /Invoices/{id} — update draft invoices
- [x] PUT /Items/{id} — update items
- [x] PUT /Accounts/{id} — update accounts
- [x] DELETE /Invoices/{id} — soft-delete (set status=DELETED)
- [x] DELETE /Contacts/{id} — archive contact
- [x] DELETE /Items/{id} — delete item

### 2. Invoice void/delete
- [x] POST to transition invoice status to VOIDED
- [x] Voiding reverses journal entries via engine
- [x] DELETE sets status to DELETED
- [x] Voided/deleted invoices filterable via where param

### 3. Bank transaction ledger integration
- [x] Creating a bank transaction posts journal entries (double-entry)
- [x] RECEIVE: debit BankAccount, credit line item accounts
- [x] SPEND: debit line item accounts, credit BankAccount
- [x] DELETE reverses journal entries

### 4. Credit note application to invoices
- [x] Endpoint to allocate credit note against an invoice
- [x] Reduces invoice AmountDue
- [x] Decrements credit note RemainingCredit
- [x] Updates invoice status to PAID if fully covered
- [x] Posts offsetting journal entry

### 5. Contact validation on transactions
- [x] Creating an invoice validates ContactID exists
- [x] Creating a bank transaction validates ContactID if provided

## Medium Value

### 6. Partial payment support
- [x] Multiple payments against a single invoice
- [x] AmountDue tracks cumulative payments
- [x] Status remains AUTHORISED until fully paid
- [x] PaymentType auto-set based on invoice type

### 7. Payment deletion/reversal
- [x] DELETE /Payments/{id} — reverse a payment
- [x] Re-credits invoice AmountDue
- [x] Reverts invoice status from PAID to AUTHORISED if needed
- [x] Reverses journal entries

### 8. Item reference in line items
- [x] Line items reference ItemCode
- [x] UnitAmount/AccountCode defaults from Item.SalesDetails or Item.PurchaseDetails

### 9. Contact balance tracking
- [x] Contact.Balances updated when invoices created/paid
- [x] AccountsReceivable/AccountsPayable totals maintained

## Lower Priority

### 10. Tax rate support
- [x] Tax rates stored and retrievable
- [x] TaxType on line items resolved to tax rate
- [x] TaxAmount calculated per line and on invoice total

### 11. Tracking categories
- [x] CRUD for tracking categories and options

### 12. Multi-currency basics
- [x] CurrencyCode on invoices used in journal entries
- [x] CurrencyRate field supported on invoices

### 13. Prepayments and overpayments
- [x] Prepayment entity type (payment before invoice)
- [x] Overpayment entity type (excess payment)
- [x] CRUD for both with RemainingCredit tracking
