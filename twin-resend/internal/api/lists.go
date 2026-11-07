package api

// listBody is Resend's list envelope for the older list endpoints, which
// return every item when limit is absent (quirk pagination-002). An empty
// list is [], never null, so SDK callers can iterate data on a fresh
// emulator.
func listBody[T any](items []T) map[string]any {
	if items == nil {
		items = []T{}
	}
	return map[string]any{"object": "list", "has_more": false, "data": items}
}
