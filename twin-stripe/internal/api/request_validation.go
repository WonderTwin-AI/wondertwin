package api

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:generate go run gen_request_schema.go -spec $STRIPE_SPEC

// Integer kinds a parameter's description gives it (request_schema_gen.go).
const (
	intAny      = iota + 1 // any integer
	intUnsigned            // described as a positive or non-negative integer
)

// requestSchema is what a route accepts, from the pinned spec.
type requestSchema struct {
	params   map[string]struct{}
	integers map[string]int
}

func setOf(names ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

// handlerOnlyParams are parameters handlers read that the dahlia spec does
// not take on that route. They predate dahlia, and refusing them would break
// the endpoint (promotion codes are created from coupon, not from dahlia's
// promotion[coupon]), so they stay accepted until the handlers move to the
// dahlia shape.
var handlerOnlyParams = map[string][]string{
	"POST /v1/checkout/sessions":   {"amount_total"},
	"POST /v1/credit_notes":        {"currency"},
	"GET /v1/credit_notes/preview": {"currency"},
	"POST /v1/promotion_codes":     {"coupon"},
	"POST /v1/quotes":              {"currency", "amount_total", "amount_subtotal"},
}

func init() {
	for key, names := range handlerOnlyParams {
		s, ok := requestSchemas[key]
		if !ok {
			panic("handlerOnlyParams: no spec route " + key)
		}
		for _, n := range names {
			s.params[n] = struct{}{}
		}
	}
}

// routeTemplate is one requestSchemas key split into its path segments.
type routeTemplate struct {
	key      string
	segments []string
	literals int
}

var routeTemplates = func() map[string][]routeTemplate {
	out := map[string][]routeTemplate{}
	for key := range requestSchemas {
		method, path, _ := strings.Cut(key, " ")
		t := routeTemplate{key: key, segments: strings.Split(path, "/")}
		for _, s := range t.segments {
			if s != "{}" {
				t.literals++
			}
		}
		out[method] = append(out[method], t)
	}
	// The most literal match wins: /v1/customers/search over /v1/customers/{}.
	for _, ts := range out {
		sort.Slice(ts, func(i, j int) bool { return ts[i].literals > ts[j].literals })
	}
	return out
}()

// requestSchemaFor finds the spec route a request is for.
func requestSchemaFor(method, path string) (requestSchema, bool) {
	segs := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for _, t := range routeTemplates[method] {
		if len(t.segments) != len(segs) {
			continue
		}
		match := true
		for i, s := range t.segments {
			if s != "{}" && s != segs[i] {
				match = false
				break
			}
		}
		if match {
			return requestSchemas[t.key], true
		}
	}
	return requestSchema{}, false
}

var arrayIndex = regexp.MustCompile(`\[\d*\]`)

// validateParamsMiddleware refuses what Stripe refuses before an endpoint
// runs: a parameter the endpoint does not take (parameter_unknown), a value
// that is not an integer where one is required (parameter_invalid_integer),
// a negative value where a positive integer is required, and a list limit
// outside 1 to 100. Routes the spec does not document are passed through.
func (h *Handler) validateParamsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schema, ok := requestSchemaFor(r.Method, r.URL.Path)
		if !ok || strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			next.ServeHTTP(w, r)
			return
		}
		_ = r.ParseForm()
		keys := make([]string, 0, len(r.Form))
		for k := range r.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			top, _, _ := strings.Cut(key, "[")
			if _, known := schema.params[top]; !known {
				writeError(w, http.StatusBadRequest, apiError{
					Type:    "invalid_request_error",
					Code:    "parameter_unknown",
					Param:   top,
					Message: "Received unknown parameter: " + top,
				})
				return
			}
			kind, isInt := schema.integers[arrayIndex.ReplaceAllString(key, "[]")]
			if !isInt {
				continue
			}
			for _, v := range r.Form[key] {
				if msg := checkInteger(key, v, kind); msg != "" {
					writeError(w, http.StatusBadRequest, apiError{
						Type:    "invalid_request_error",
						Code:    "parameter_invalid_integer",
						Param:   key,
						Message: msg,
					})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// checkInteger returns Stripe's message for a value the parameter refuses, or "".
func checkInteger(key, v string, kind int) string {
	if v == "" {
		// An empty value unsets an optional parameter on /v1.
		return ""
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return "Invalid integer: " + v
	}
	if key == "limit" && (n < 1 || n > 100) {
		if n < 1 {
			return "This value must be greater than or equal to 1."
		}
		return "This value must be less than or equal to 100."
	}
	if kind == intUnsigned && n < 0 {
		return "Invalid positive integer"
	}
	return ""
}
