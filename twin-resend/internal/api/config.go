package api

import (
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twinkit/admin"
)

// The emulator's own /admin/config keys, alongside the kit's.
const (
	// strictSendersKey turns on Resend's full sender-domain rules; see
	// senderCheck.
	strictSendersKey = "strict_sender_domains"
	// domainVerificationKey sets where POST /domains/{id}/verify leaves a
	// domain: verified (the default), pending or failed.
	domainVerificationKey = "domain_verification"
	// rateLimitKey is the requests allowed per one-second window; 0, the
	// default, never answers 429.
	rateLimitKey = "rate_limit"
)

// ConfigProvider extends the kit's runtime config with the emulator's own
// switches, readable and settable through /admin/config.
func (h *Handler) ConfigProvider(base admin.ConfigProvider) admin.ConfigProvider {
	return &resendConfig{base: base, h: h}
}

type resendConfig struct {
	base admin.ConfigProvider
	h    *Handler
}

func (c *resendConfig) GetConfig() map[string]any {
	out := c.base.GetConfig()
	out[strictSendersKey] = c.h.strictSenders.Load()
	out[domainVerificationKey] = c.h.verifyOutcome()
	out[rateLimitKey] = c.h.limiter.limit()
	return out
}

// UpdateConfig validates every emulator key before applying any of them, then
// hands the rest to the kit.
func (c *resendConfig) UpdateConfig(updates map[string]any) error {
	rest := make(map[string]any, len(updates))
	var apply []func()
	for k, v := range updates {
		switch k {
		case strictSendersKey:
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("%s must be a boolean", k)
			}
			apply = append(apply, func() { c.h.strictSenders.Store(b) })
		case domainVerificationKey:
			s, ok := v.(string)
			if !ok || !validVerifyOutcome(s) {
				return fmt.Errorf("%s must be one of verified, pending, failed", k)
			}
			apply = append(apply, func() { c.h.verifyMode.Store(s) })
		case rateLimitKey:
			f, ok := v.(float64)
			if !ok || f < 0 || f != float64(int(f)) {
				return fmt.Errorf("%s must be a whole number of requests per second, 0 for off", k)
			}
			apply = append(apply, func() { c.h.limiter.configure(int(f)) })
		default:
			rest[k] = v
		}
	}
	if len(rest) > 0 {
		if err := c.base.UpdateConfig(rest); err != nil {
			return err
		}
	}
	for _, f := range apply {
		f()
	}
	return nil
}
