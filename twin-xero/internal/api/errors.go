package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// problem is the RFC 7807 body Xero answers an unauthenticated call with
// (quirk auth-005). Its fields are capitalised, Type is null, and Instance is
// the call's correlation id.
type problem struct {
	Type       *string  `json:"Type"`
	Title      string   `json:"Title"`
	Status     int      `json:"Status"`
	Detail     string   `json:"Detail"`
	Instance   string   `json:"Instance"`
	Extensions struct{} `json:"Extensions"`
}

// writeProblem answers with a problem-details body, as JSON whatever Accept says.
func (h *Handler) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	instance := twincore.CorrelationIDFromContext(r.Context())
	if instance == "" {
		instance = h.store.NewID()
	}
	twincore.JSON(w, status, problem{Title: title, Status: status, Detail: detail, Instance: instance})
}

// writeUnauthorized answers 401. A call that sent a token is told it was
// invalid; one that sent none gets the bare Bearer challenge.
func (h *Handler) writeUnauthorized(w http.ResponseWriter, r *http.Request, tokenSent bool) {
	challenge := "Bearer"
	if tokenSent {
		challenge = `Bearer error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	h.writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "AuthenticationUnsuccessful")
}

// validationElement is one submitted object and the reasons it was refused.
type validationElement struct {
	Object   any
	Messages []string
}

// writeValidationException answers 400 with Xero's ValidationException body
// (quirk err-001). Each element echoes the submitted object and adds its
// ValidationErrors.
func writeValidationException(w http.ResponseWriter, elements ...validationElement) {
	out := make([]map[string]any, 0, len(elements))
	for _, e := range elements {
		el := map[string]any{}
		if b, err := json.Marshal(e.Object); err == nil {
			_ = json.Unmarshal(b, &el)
		}
		if el == nil { // a nil Object marshals to null and unmarshals to a nil map
			el = map[string]any{}
		}
		errs := make([]map[string]string, 0, len(e.Messages))
		for _, m := range e.Messages {
			errs = append(errs, map[string]string{"Message": m})
		}
		el["ValidationErrors"] = errs
		out = append(out, el)
	}
	twincore.JSON(w, http.StatusBadRequest, map[string]any{
		"ErrorNumber": 10,
		"Type":        "ValidationException",
		"Message":     "A validation exception occurred",
		"Elements":    out,
	})
}

// badValueError is a date the body carried that Xero cannot read. It keeps the
// raw JSON of the value so the answer can name the field that held it.
type badValueError struct{ Raw []byte }

func (e *badValueError) Error() string { return "invalid date " + string(e.Raw) }

// writeBadBody answers a body that did not decode. JSON that is not JSON stays a
// plain 400 problem. A value of the wrong type or a date that cannot be read is a
// ValidationException that names the field, as every other refused value is
// (audit finding F3). The exact words are a guess; Xero's are not observed.
func (h *Handler) writeBadBody(w http.ResponseWriter, r *http.Request, raw []byte, err error) {
	var typeErr *json.UnmarshalTypeError
	var valueErr *badValueError
	switch {
	case errors.As(err, &typeErr):
		writeValidationException(w, validationElement{Messages: []string{lastKey(typeErr.Field) + " is not valid"}})
	case errors.As(err, &valueErr):
		field := fieldHolding(raw, valueErr.Raw)
		if field == "" {
			field = "Date"
		}
		writeValidationException(w, validationElement{Messages: []string{field + " is not a valid date"}})
	default:
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
	}
}

// lastKey is the last name in a dotted field path.
func lastKey(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}

// fieldHolding finds the key under which the body carries the value, searching
// every nesting level and taking the first key in name order, so the answer does
// not vary from call to call.
func fieldHolding(body, value []byte) string {
	var tree any
	if json.Unmarshal(body, &tree) != nil {
		return ""
	}
	var walk func(v any) string
	walk = func(v any) string {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if enc, err := json.Marshal(t[k]); err == nil && bytes.Equal(enc, value) {
					return k
				}
			}
			for _, k := range keys {
				if f := walk(t[k]); f != "" {
					return f
				}
			}
		case []any:
			for _, e := range t {
				if f := walk(e); f != "" {
					return f
				}
			}
		}
		return ""
	}
	return walk(tree)
}
