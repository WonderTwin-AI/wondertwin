package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

//go:generate go run gen_response_schema.go -spec $STRIPE_SPEC

// Fill rules for a property the handler left out (response_schema_gen.go).
const (
	fillNone        = iota // no unset value; left as the handler wrote it
	fillNull               // nullable: returned as null
	fillEmptyMap           // a string map such as metadata: returned as {}
	fillEmptyArray         // a non-nullable array: returned as []
	fillEmptyObject        // an embedded sub-object whose every field fills
)

// responseProp is one property of a response schema.
type responseProp struct {
	name   string
	fill   int
	nested string // the embedded sub-object schema, if any
}

// fillResponse adds every property Stripe returns but the handler left out,
// with the value Stripe returns when it is unset. Stripe returns unset
// nullable fields as null, never omitted (decision 9), and the official SDKs
// read them as attributes: stripe-python raises AttributeError on an omitted
// one. The Go types use omitempty throughout, so this is done once on the
// encoded response rather than per type.
//
// Objects are found by their "object" value, and embedded sub-objects (such
// as a customer's invoice_settings) by the property that holds them. Deleted
// objects and lists are left as they are; list items are filled.
func fillResponse(v any) {
	fillValue(v, "")
}

func fillValue(v any, schema string) {
	switch t := v.(type) {
	case map[string]any:
		fillObject(t, schema)
	case []any:
		for _, item := range t {
			fillValue(item, schema)
		}
	}
}

func fillObject(m map[string]any, schema string) {
	if obj, ok := m["object"].(string); ok {
		if deleted, _ := m["deleted"].(bool); deleted {
			return
		}
		if s, ok := objectSchemas[obj]; ok {
			schema = s
		}
	}
	props, ok := responseSchemas[schema]
	if !ok {
		// An unknown shape (a list, an event's previous_attributes): fill
		// the objects inside it.
		for k, child := range m {
			if k != "metadata" {
				fillValue(child, "")
			}
		}
		return
	}
	known := make(map[string]string, len(props))
	for _, p := range props {
		known[p.name] = p.nested
		if _, present := m[p.name]; present {
			continue
		}
		switch p.fill {
		case fillNull:
			m[p.name] = nil
		case fillEmptyMap:
			m[p.name] = map[string]any{}
		case fillEmptyArray:
			m[p.name] = []any{}
		case fillEmptyObject:
			m[p.name] = map[string]any{}
		}
	}
	for k, child := range m {
		if k == "metadata" {
			continue
		}
		fillValue(child, known[k])
	}
}

// fillMiddleware applies fillResponse to every successful JSON response.
func (h *Handler) fillMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := newExpandBuffer()
		next.ServeHTTP(buf, r)
		if buf.statusCode < 200 || buf.statusCode >= 300 ||
			!strings.Contains(buf.header.Get("Content-Type"), "application/json") {
			flushBuffer(w, buf)
			return
		}
		var body any
		dec := json.NewDecoder(bytes.NewReader(buf.body.Bytes()))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			flushBuffer(w, buf)
			return
		}
		fillResponse(body)
		encoded, err := json.Marshal(body)
		if err != nil {
			flushBuffer(w, buf)
			return
		}
		encoded = append(encoded, '\n')
		copyHeader(w.Header(), buf.header)
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		w.WriteHeader(buf.statusCode)
		_, _ = w.Write(encoded)
	})
}
