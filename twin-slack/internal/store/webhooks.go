package store

import (
	"crypto/rand"
	"math/big"
)

// webhookAlphabet is the characters of a minted service ID and secret. Slack's
// documented examples use upper-case letters and digits; the real alphabet and
// lengths are unverified (quirk events-018).
const webhookAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Lengths of a minted service ID (after its B) and secret, as in Slack's
// documented example URL.
const (
	webhookServiceLen = 10
	webhookSecretLen  = 24
)

// MintIncomingWebhook records a new incoming webhook for channel, owned by the
// app and bot given, and returns it. Its service ID and secret are random, so
// a URL cannot be guessed.
func (s *MemoryStore) MintIncomingWebhook(channel, appID, botID string) IncomingWebhook {
	wh := IncomingWebhook{
		ID:      s.IncomingWebhooks.NextID(),
		Team:    s.Team.ID,
		Service: "B" + randomString(webhookServiceLen),
		Secret:  randomString(webhookSecretLen),
		Channel: channel,
		AppID:   appID,
		BotID:   botID,
		Created: s.Clock.Now().Unix(),
	}
	s.IncomingWebhooks.Set(wh.ID, wh)
	return wh
}

func randomString(n int) string {
	out := make([]byte, n)
	max := big.NewInt(int64(len(webhookAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		out[i] = webhookAlphabet[v.Int64()]
	}
	return string(out)
}
