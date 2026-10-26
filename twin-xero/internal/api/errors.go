package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
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

// badValueError is a date the body carried that Xero cannot read. Field is the
// key that held it, filled in by decodeBody.
type badValueError struct{ Field string }

func (e *badValueError) Error() string { return "invalid date in " + e.Field }

// writeBadBody answers a body that did not decode. JSON that is not JSON stays a
// plain 400 problem. A value of the wrong type or a date that cannot be read is a
// ValidationException that names the field, as every other refused value is
// (audit finding F3). The exact words are a guess; Xero's are not observed.
func (h *Handler) writeBadBody(w http.ResponseWriter, r *http.Request, err error) {
	var typeErr *json.UnmarshalTypeError
	var valueErr *badValueError
	switch {
	case errors.As(err, &typeErr):
		writeValidationException(w, validationElement{Messages: []string{lastKey(typeErr.Field) + " is not valid"}})
	case errors.As(err, &valueErr):
		field := valueErr.Field
		if field == "" {
			field = "Date"
		}
		writeValidationException(w, validationElement{Messages: []string{field + " is not a valid date"}})
	default:
		h.writeProblem(w, r, http.StatusBadRequest, "Bad Request", "The request body is not valid JSON")
	}
}

// decodeBody is json.Unmarshal that, when a date cannot be read, says which key
// held it. encoding/json drops that, so the body is decoded again one key at a
// time into a fresh v, in document order, and the first key whose own decode
// fails is the one named.
func decodeBody(raw []byte, v any) error {
	err := json.Unmarshal(raw, v)
	var bad *badValueError
	if !errors.As(err, &bad) {
		return err
	}
	typ := reflect.TypeOf(v).Elem()
	fails := func(b []byte) bool {
		var be *badValueError
		return errors.As(json.Unmarshal(b, reflect.New(typ).Interface()), &be)
	}
	bad.Field = badKey(raw, func(b []byte) []byte { return b }, fails)
	return err
}

// badKey descends val in document order. wrap puts a fragment back at val's
// place in the body, so each probe is a whole body carrying just that fragment.
func badKey(val []byte, wrap func([]byte) []byte, fails func([]byte) bool) string {
	dec := json.NewDecoder(bytes.NewReader(val))
	open, err := dec.Token()
	if err != nil {
		return ""
	}
	for dec.More() {
		key := ""
		if open == json.Delim('{') {
			t, err := dec.Token()
			if err != nil {
				return ""
			}
			key, _ = t.(string)
		}
		var child json.RawMessage
		if dec.Decode(&child) != nil {
			return ""
		}
		inner := func(b []byte) []byte { return wrap(append(append([]byte("["), b...), ']')) }
		if open == json.Delim('{') {
			quoted, _ := json.Marshal(key)
			inner = func(b []byte) []byte {
				return wrap(append(append(append([]byte("{"), quoted...), ':'), append(b, '}')...))
			}
		}
		if !fails(inner(child)) {
			continue
		}
		if deeper := badKey(child, inner, fails); deeper != "" {
			return deeper
		}
		return key
	}
	return ""
}

// lastKey is the last name in a dotted field path.
func lastKey(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}
