package ledger

// DocumentTransitionContext builds the canonical Context map for OnDocumentStateTransition.
func DocumentTransitionContext(from, to DocumentStatus, docType DocumentType, lineCount int) map[string]any {
	return map[string]any{
		"from":          string(from),
		"to":            string(to),
		"document_type": string(docType),
		"line_count":    lineCount,
	}
}

// JournalEntryContext builds the canonical Context map for OnJournalEntryCreated.
func JournalEntryContext(entryCount, accountsTouched int, totalDebitsNonzero bool) map[string]any {
	return map[string]any{
		"entry_count":          entryCount,
		"accounts_touched":     accountsTouched,
		"total_debits_nonzero": totalDebitsNonzero,
	}
}

// PaymentAppliedContext builds the canonical Context map for OnPaymentApplied.
func PaymentAppliedContext(paymentType string, fullyPaid, overpayment bool) map[string]any {
	return map[string]any{
		"payment_type": paymentType,
		"fully_paid":   fullyPaid,
		"overpayment":  overpayment,
	}
}

// ValidateDocumentContext builds the canonical Context map for ValidateDocument.
func ValidateDocumentContext(docType DocumentType, valid bool, errorType string) map[string]any {
	ctx := map[string]any{
		"document_type": string(docType),
		"valid":         valid,
	}
	if errorType != "" {
		ctx["error_type"] = errorType
	}
	return ctx
}
