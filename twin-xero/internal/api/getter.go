package api

import (
	"reflect"
	"strings"
	"time"
)

// fieldGetter reads a record's fields by their JSON names for a where
// expression. An optional field (one tagged omitempty) holding its zero value
// reads as null, as an unset field does in Xero; a name the record does not have
// is not found. Only scalar fields and dates are readable.
func fieldGetter(record any) Getter {
	v := reflect.ValueOf(record)
	fields := map[string]int{}
	optional := map[string]bool{}
	for i := 0; i < v.NumField(); i++ {
		name, opts, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		fields[name], optional[name] = i, strings.Contains(opts, "omitempty")
	}
	return func(path string) (any, bool) {
		i, ok := fields[path]
		if !ok {
			return nil, false
		}
		f := v.Field(i)
		if optional[path] && f.IsZero() {
			return nil, true
		}
		switch x := f.Interface().(type) {
		case string, bool, int, int64, float64, time.Time:
			return x, true
		}
		return nil, false
	}
}
