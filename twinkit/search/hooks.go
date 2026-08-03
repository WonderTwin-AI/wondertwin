package search

import "context"

// SearchHooks defines extension points for platform-specific behavior.
type SearchHooks interface {
	// ValidateRecord is called before adding or updating a record.
	ValidateRecord(ctx context.Context, indexName string, record *Record) error

	// OnRecordSaved is called after a record is added or updated.
	OnRecordSaved(ctx context.Context, indexName string, record *Record) error

	// OnRecordDeleted is called after a record is deleted.
	OnRecordDeleted(ctx context.Context, indexName, objectID string) error

	// OnSearch is called after a search is executed (for analytics/logging).
	OnSearch(ctx context.Context, indexName string, params *SearchParams, result *SearchResult) error
}

// NoOpSearchHooks provides default no-op implementations.
type NoOpSearchHooks struct{}

func (NoOpSearchHooks) ValidateRecord(_ context.Context, _ string, _ *Record) error { return nil }
func (NoOpSearchHooks) OnRecordSaved(_ context.Context, _ string, _ *Record) error  { return nil }
func (NoOpSearchHooks) OnRecordDeleted(_ context.Context, _, _ string) error        { return nil }
func (NoOpSearchHooks) OnSearch(_ context.Context, _ string, _ *SearchParams, _ *SearchResult) error {
	return nil
}
