package api

import (
	"fmt"
	"net/mail"
	"strings"
	"sync/atomic"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
)

// strictSendersKey is the admin config key that turns on Resend's full
// sender-domain rules.
const strictSendersKey = "strict_sender_domains"

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

// ConfigProvider extends the kit's runtime config with the emulator's own
// strict_sender_domains switch, readable and settable through /admin/config.
func (h *Handler) ConfigProvider(base admin.ConfigProvider) admin.ConfigProvider {
	return &resendConfig{base: base, strict: &h.strictSenders}
}

type resendConfig struct {
	base   admin.ConfigProvider
	strict *atomic.Bool
}

func (c *resendConfig) GetConfig() map[string]any {
	out := c.base.GetConfig()
	out[strictSendersKey] = c.strict.Load()
	return out
}

func (c *resendConfig) UpdateConfig(updates map[string]any) error {
	rest := make(map[string]any, len(updates))
	var strict *bool
	for k, v := range updates {
		if k != strictSendersKey {
			rest[k] = v
			continue
		}
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("%s must be a boolean", strictSendersKey)
		}
		strict = &b
	}
	if len(rest) > 0 {
		if err := c.base.UpdateConfig(rest); err != nil {
			return err
		}
	}
	if strict != nil {
		c.strict.Store(*strict)
	}
	return nil
}
