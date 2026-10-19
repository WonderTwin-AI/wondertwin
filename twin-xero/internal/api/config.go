package api

import (
	"fmt"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// Config serves GET and PUT /admin/config: the kit's runtime configuration plus
// strict_auth, which makes the emulator refuse tokens it did not issue.
type Config struct {
	twin  *twincore.Twin
	store *store.MemoryStore
}

// NewConfig returns the config provider for the admin handler.
func NewConfig(twin *twincore.Twin, s *store.MemoryStore) *Config {
	return &Config{twin: twin, store: s}
}

// GetConfig returns the kit's configuration with strict_auth added.
func (c *Config) GetConfig() map[string]any {
	cfg := c.twin.GetConfig()
	cfg["strict_auth"] = c.store.StrictAuth.Load()
	return cfg
}

// UpdateConfig applies strict_auth and hands every other key to the kit, which
// rejects an unknown one. Nothing is applied if any key is refused.
func (c *Config) UpdateConfig(updates map[string]any) error {
	rest := make(map[string]any, len(updates))
	for k, v := range updates {
		if k != "strict_auth" {
			rest[k] = v
		}
	}
	strict, set := updates["strict_auth"]
	on, isBool := strict.(bool)
	if set && !isBool {
		return fmt.Errorf("strict_auth must be a boolean")
	}
	if err := c.twin.UpdateConfig(rest); err != nil {
		return err
	}
	if set {
		c.store.StrictAuth.Store(on)
	}
	return nil
}
