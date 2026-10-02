package api

import "testing"

// SetMaxUploadBytes lowers the upload limit for the duration of a test.
func SetMaxUploadBytes(t *testing.T, n int64) {
	t.Helper()
	old := maxUploadBytes
	maxUploadBytes = n
	t.Cleanup(func() { maxUploadBytes = old })
}
