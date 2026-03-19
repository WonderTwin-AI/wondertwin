# twin-qbo Behavior Backlog

Work items to deepen twin-qbo from "routes exist" to "behaviors work".

## High Value

### 1. Journal entry reversals on void/delete
- [x] Voiding an invoice reverses its AR/Income journal entries
- [x] Voiding a bill reverses its Expense/AP journal entries
- [x] Voiding a payment reverses its Bank/AR journal entries
- [x] Voiding a bill payment reverses its AP/Bank journal entries

### 2. Sparse update support for all entities
- [x] Invoices support sparse=true (merge fields instead of replacing)
- [x] Bills support sparse=true
- [x] Vendors support sparse=true
- [x] Items support sparse=true
- [x] Accounts support sparse=true

### 3. Estimate → invoice conversion
- [x] POST endpoint to convert an estimate into an invoice
- [x] Copies line items, customer ref, and terms from estimate
- [x] Transitions estimate status to Closed
- [x] Validate estimate status transitions: Pending → Accepted → Closed

### 4. Customer/vendor balance tracking
- [x] Creating an invoice updates customer Balance field
- [x] Paying an invoice reduces customer Balance
- [x] Creating a bill updates vendor Balance
- [x] Paying a bill reduces vendor Balance
- [x] Voiding transactions adjusts balances

### 5. Account balance maintenance
- [x] Account.CurrentBalance updated when journal entries post
- [x] Account.CurrentBalance updated when journal entries reverse

## Medium Value

### 6. Credit memo application to invoices
- [x] Applying a credit memo reduces invoice Balance
- [x] Tracks CreditMemo.RemainingCreditAmt
- [x] Generates reversing journal entry for applied amount

### 7. Multi-currency basics
- [x] CurrencyRef on transactions used in journal entries
- [x] HomeTotalAmt calculated from exchange rate
- [x] ExchangeRate field respected on create

### 8. Tax rate lookup
- [x] TaxCode references resolve to tax rates
- [x] Tax amount calculated on invoice/bill line items
- [x] TxnTaxDetail populated automatically

### 9. Batch endpoint hardening
- [x] Proper error handling per operation (not all-or-nothing)
- [x] Validate entity types and required fields
- [x] Return individual operation results

## Lower Priority

### 10. Aging reports
- [x] Aged receivables by customer (current, 1-30, 31-60, 61-90, 90+)
- [x] Aged payables by vendor

### 11. Discount/markup handling
- [x] Discount line items on invoices
- [x] DiscountAmt and DiscountRate fields

### 12. Recurring transactions
- [x] Templates for invoices/bills
- [x] CRUD for recurring transaction templates
