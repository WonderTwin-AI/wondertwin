package api

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// sharedSenderDomain is the domain of Resend's shared onboarding sender.
const sharedSenderDomain = "resend.dev"

// senderCheck applies Resend's sender-domain rules (quirk lifecycle-007) and
// returns the 403 message, or "" when the send may go ahead.
//
// By default only a domain created through POST /domains and not yet verified
// is refused; a domain the emulator has never seen is accepted, so tests need
// no setup. With strict_sender_domains on, every sender domain must exist and
// be verified, and the shared resend.dev sender may send only to resend.dev
// test addresses, since the emulator has no account owner to send to.
func (h *Handler) senderCheck(req sendEmailRequest) string {
	domain := senderDomain(req.From)
	strict := h.strictSenders.Load()
	if domain == sharedSenderDomain {
		if !strict {
			return ""
		}
		for _, list := range [][]string{req.To, req.CC, req.BCC} {
			for _, addr := range list {
				if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(addr)), "@"+sharedSenderDomain) {
					return "You can only send testing emails to your own email address. To send emails to other recipients, please verify a domain at resend.com/domains, and change the `from` address to an email using this domain."
				}
			}
		}
		return ""
	}
	known, verified := h.domainStatus(domain)
	if verified || (!known && !strict) {
		return ""
	}
	return fmt.Sprintf("The %s domain is not verified. Please, add and verify your domain on https://resend.com/domains", domain)
}

// domainStatus reports whether a domain was created in the emulator and
// whether any domain of that name is verified.
func (h *Handler) domainStatus(name string) (known, verified bool) {
	for _, d := range h.store.Domains.List() {
		if strings.EqualFold(d.Name, name) {
			known = true
			if d.Status == store.DomainStatusVerified {
				verified = true
			}
		}
	}
	return known, verified
}

// senderDomain returns the lower-cased domain of a from field, which may be a
// bare address or "Name <address>".
func senderDomain(from string) string {
	addr := from
	if parsed, err := mail.ParseAddress(from); err == nil {
		addr = parsed.Address
	}
	_, domain, _ := strings.Cut(addr, "@")
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ">")))
}
