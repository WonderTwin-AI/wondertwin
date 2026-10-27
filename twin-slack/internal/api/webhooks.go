package api

import "github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"

// webhooksPath is where the emulator serves incoming webhook URLs, which on
// Slack are under hooks.slack.com/services/.
const webhooksPath = "/services/"

// webhookURL is the URL an app posts to for wh, under base, the scheme and
// host the client called (see origin). Slack's host is hooks.slack.com, so the
// host here is a declared difference. The path is Slack's documented form
// /services/{team}/{service}/{secret}.
func webhookURL(base string, wh store.IncomingWebhook) string {
	return base + webhooksPath + wh.Team + "/" + wh.Service + "/" + wh.Secret
}
