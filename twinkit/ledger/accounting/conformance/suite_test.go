package conformance

import "testing"

// TestConformanceSuite runs the full conformance suite against
// the standard test engine. This verifies the suite itself works.
func TestConformanceSuite(t *testing.T) {
	RunWithFactory(t, NewTestEngine)
}
