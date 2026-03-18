package api

import (
	"fmt"
	"strconv"
)

// ValidateSyncToken checks that the provided SyncToken matches the current one.
// Returns an error if they don't match.
func ValidateSyncToken(current, provided string) error {
	if current != provided {
		return fmt.Errorf("stale SyncToken: current=%s provided=%s", current, provided)
	}
	return nil
}

// IncrementSyncToken increments a string-integer SyncToken.
func IncrementSyncToken(token string) string {
	n, err := strconv.Atoi(token)
	if err != nil {
		return "1"
	}
	return strconv.Itoa(n + 1)
}
