package search

// RecordSavedContext builds the canonical Context map for OnRecordSaved.
func RecordSavedContext(indexName string, attributeCount int) map[string]any {
	return map[string]any{
		"index_name":      indexName,
		"attribute_count": attributeCount,
	}
}

// SearchContext builds the canonical Context map for OnSearch.
func SearchContext(indexName string, hitCount int, hasFilters, hasFacets bool) map[string]any {
	return map[string]any{
		"index_name":  indexName,
		"hit_count":   hitCount,
		"has_filters": hasFilters,
		"has_facets":  hasFacets,
	}
}

// RecordDeletedContext builds the canonical Context map for OnRecordDeleted.
func RecordDeletedContext(indexName string) map[string]any {
	return map[string]any{
		"index_name": indexName,
	}
}
