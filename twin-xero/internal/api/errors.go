package api

import (
	"encoding/json"
	"net/http"

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
