package api

import "testing"

// SetMaxUploadBytes lowers the upload limit for the duration of a test.
func SetMaxUploadBytes(t *testing.T, n int64) {
	t.Helper()
	old := maxUploadBytes
	maxUploadBytes = n
	t.Cleanup(func() { maxUploadBytes = old })
}

// WebhookURL exposes webhookURL to the tests.
var WebhookURL = webhookURL

// IssueSocketTicket issues a Socket Mode ticket, as apps.connections.open will.
func (h *Handler) IssueSocketTicket() string { return h.sockets.issueTicket() }

// SocketPath is where Socket Mode URLs point.
const SocketPath = socketPath
