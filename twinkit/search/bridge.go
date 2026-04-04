package search

import (
	"context"

	"github.com/wondertwin-ai/wondertwin/twinkit/domainevents"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// WithTelemetry returns an Option that wraps the engine's hooks to automatically
// emit DomainEvent telemetry when hooks fire.
func WithTelemetry(emitter domainevents.Emitter) Option {
	return func(e *Engine) {
		inner := e.hooks
		if inner == nil {
			inner = NoOpSearchHooks{}
		}
		e.hooks = &telemetryBridge{emitter: emitter, inner: inner}
	}
}

type telemetryBridge struct {
	emitter domainevents.Emitter
	inner   SearchHooks
}

func (b *telemetryBridge) ValidateRecord(ctx context.Context, indexName string, record *Record) error {
	return b.inner.ValidateRecord(ctx, indexName, record)
}

func (b *telemetryBridge) OnRecordSaved(ctx context.Context, indexName string, record *Record) error {
	attrCount := 0
	if record.Attributes != nil {
		attrCount = len(record.Attributes)
	}
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "search",
			Hook:       "OnRecordSaved",
			Resource:   indexName,
			Action:     "create",
			EntityType: "record",
			EntityID:   record.ObjectID,
			Context:    RecordSavedContext(indexName, attrCount),
		},
	)
	return b.inner.OnRecordSaved(ctx, indexName, record)
}

func (b *telemetryBridge) OnRecordDeleted(ctx context.Context, indexName, objectID string) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "search",
			Hook:       "OnRecordDeleted",
			Resource:   indexName,
			Action:     "delete",
			EntityType: "record",
			EntityID:   objectID,
			Context:    RecordDeletedContext(indexName),
		},
	)
	return b.inner.OnRecordDeleted(ctx, indexName, objectID)
}

func (b *telemetryBridge) OnSearch(ctx context.Context, indexName string, params *SearchParams, result *SearchResult) error {
	b.emitter.EmitDomainEvent(
		twincore.CorrelationIDFromContext(ctx),
		domainevents.DomainEvent{
			Engine:     "search",
			Hook:       "OnSearch",
			Resource:   indexName,
			Action:     "read",
			EntityType: "search_query",
			Context: SearchContext(
				indexName,
				result.NbHits,
				params.Filters != "" || len(params.FacetFilters) > 0,
				len(params.Facets) > 0,
			),
		},
	)
	return b.inner.OnSearch(ctx, indexName, params, result)
}
