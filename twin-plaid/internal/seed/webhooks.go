package seed

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed webhook_bodies.json
var webhookBodiesJSON []byte

var (
	webhookOnce    sync.Once
	webhookSamples map[string]json.RawMessage
)

// WebhookSample returns a copy of Plaid's documented sample body for a
// webhook, keyed "TYPE.CODE" such as "ITEM.LOGIN_REPAIRED". Provenance and the
// codes with no published sample are in webhook_bodies.json.
func WebhookSample(key string) (map[string]any, bool) {
	webhookOnce.Do(func() {
		var f struct {
			Samples map[string]json.RawMessage `json:"samples"`
		}
		if err := json.Unmarshal(webhookBodiesJSON, &f); err != nil {
			panic("seed: webhook_bodies.json: " + err.Error())
		}
		webhookSamples = f.Samples
	})
	raw, ok := webhookSamples[key]
	if !ok {
		return nil, false
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return body, true
}
