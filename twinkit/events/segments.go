package events

import (
	"fmt"
	"strings"
)

// MatchSegment returns all user profiles that match the given segment filter.
func (e *Engine) MatchSegment(filter *SegmentFilter) []*UserProfile {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matched []*UserProfile
	for _, profile := range e.profiles {
		if e.profileMatchesFilter(profile, filter) {
			matched = append(matched, profile)
		}
	}
	return matched
}

// profileMatchesFilter checks if a profile matches all criteria in the filter.
func (e *Engine) profileMatchesFilter(profile *UserProfile, filter *SegmentFilter) bool {
	// Check property filters (AND logic).
	for _, pf := range filter.PropertyFilters {
		if !matchProperty(profile.Properties, pf) {
			return false
		}
	}

	// Check event filter.
	if filter.EventFilter != nil {
		count := 0
		for _, evt := range e.events {
			if evt.DistinctID == profile.DistinctID && evt.EventName == filter.EventFilter.EventName {
				count++
			}
		}
		minCount := filter.EventFilter.MinCount
		if minCount == 0 {
			minCount = 1
		}
		if count < minCount {
			return false
		}
	}

	return true
}

// matchProperty evaluates a single property filter against a properties map.
func matchProperty(props map[string]any, filter PropertyFilter) bool {
	val, exists := props[filter.Key]

	switch filter.Operator {
	case "exists":
		return exists
	case "not_exists":
		return !exists
	case "eq":
		return exists && fmt.Sprintf("%v", val) == fmt.Sprintf("%v", filter.Value)
	case "neq":
		return !exists || fmt.Sprintf("%v", val) != fmt.Sprintf("%v", filter.Value)
	case "contains":
		if !exists {
			return false
		}
		return strings.Contains(fmt.Sprintf("%v", val), fmt.Sprintf("%v", filter.Value))
	case "gt", "lt", "gte", "lte":
		return compareNumeric(val, filter.Value, filter.Operator)
	default:
		return false
	}
}

// compareNumeric compares two values numerically.
func compareNumeric(a, b any, op string) bool {
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if !aOk || !bOk {
		return false
	}
	switch op {
	case "gt":
		return af > bf
	case "lt":
		return af < bf
	case "gte":
		return af >= bf
	case "lte":
		return af <= bf
	default:
		return false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}
