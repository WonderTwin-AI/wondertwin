# twin-qbo Behavior Backlog

Work items to deepen twin-qbo from "routes exist" to "behaviors work".

## High Value

### 1. Journal entry reversals on void/delete
- [ ] Voiding an invoice reverses its AR/Income journal entries
- [ ] Voiding a bill reverses its Expense/AP journal entries
- [ ] Voiding a payment reverses its Bank/AR journal entries
- [ ] Voiding a bill payment reverses its AP/Bank journal entries

Currently voiding sets totals to 0 but leaves stale journal entries, breaking the balance sheet.

### 2. Sparse update support for all entities
- [ ] Invoices support sparse=true (merge fields instead of replacing)
- [ ] Bills support sparse=true
- [ ] Vendors support sparse=true
- [ ] Items support sparse=true
- [ ] Accounts support sparse=true

Currently only customers use sparse merge; other entities lose unset fields on update.

### 3. Estimate → invoice conversion
- [ ] POST endpoint or parameter to convert an estimate into an invoice
- [ ] Copies line items, customer ref, and terms from estimate
- [ ] Transitions estimate status to Closed/Invoiced
- [ ] Validate estimate status transitions: Pending → Accepted → Closed

Estimates are currently dead ends with no workflow.

### 4. Customer/vendor balance tracking
- [ ] Creating an invoice updates customer Balance field
- [ ] Paying an invoice reduces customer Balance
- [ ] Creating a bill updates vendor Balance
- [ ] Paying a bill reduces vendor Balance
- [ ] Voiding transactions adjusts balances

Currently Balance on Customer/Vendor objects is never updated.

### 5. Account balance maintenance
- [ ] Account.CurrentBalance updated when journal entries post
- [ ] Account.CurrentBalance updated when journal entries reverse

Currently only the trial balance report (via engine) reflects account balances.

## Medium Value

### 6. Credit memo application to invoices
- [ ] Applying a credit memo reduces invoice Balance
- [ ] Tracks CreditMemo.RemainingCreditAmt
- [ ] Generates reversing journal entry for applied amount

### 7. Multi-currency basics
- [ ] CurrencyRef on transactions used in journal entries
- [ ] HomeTotalAmt calculated from exchange rate
- [ ] ExchangeRate field respected on create

### 8. Tax rate lookup
- [ ] TaxCode references resolve to tax rates
- [ ] Tax amount calculated on invoice/bill line items
- [ ] TxnTaxDetail populated automatically

### 9. Batch endpoint hardening
- [ ] Proper error handling per operation (not all-or-nothing)
- [ ] Validate entity types and required fields
- [ ] Return individual operation results

## Lower Priority

### 10. Aging reports
- [ ] Aged receivables by customer (current, 1-30, 31-60, 61-90, 90+)
- [ ] Aged payables by vendor

### 11. Discount/markup handling
- [ ] Discount line items on invoices
- [ ] DiscountAmt and DiscountRate fields

### 12. Recurring transactions
- [ ] Templates for invoices/bills
- [ ] Admin endpoint to trigger scheduled generation
