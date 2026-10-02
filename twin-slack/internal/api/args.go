package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Slack accepts a Web API call's arguments as a query string, as a POST body in
// one of four content types, or as a mix of query string and form body
// (docs.slack.dev apis/web-api, "POST bodies"). argsMiddleware decodes them once,
// before any handler runs, so every method sees the same arguments whichever
// way the client sent them. Handlers read them through parseJSON, which fills a
// request struct from the decoded arguments.

// maxArgsBody bounds how much of a request body is read.
const maxArgsBody = 32 << 20

type argsKey struct{}

// slackArgs holds the decoded arguments of one call.
type slackArgs struct {
	// values holds query, form, multipart and text/plain arguments. Body values
	// come first, so Get returns the body's value when a key is in both.
	values url.Values
	// body holds only the arguments that arrived in a form, multipart or
	// text/plain POST body. A token may be sent there, but not in the query
	// string and not in a JSON body.
	body url.Values
	// json holds the body of an application/json request, nil otherwise. When
	// it is set the query string is not used for arguments: Slack's docs say to
	// choose one approach per request.
	json []byte
	// err is set when the request could not be decoded. It is reported after
	// authentication by argsErrorMiddleware.
	err *argError
}

// argError is a decoding failure carrying the Slack error code to return.
type argError struct{ code string }

func (e *argError) Error() string { return e.code }

// argsMiddleware decodes the call's arguments and stores them on the request.
func argsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := decodeArgs(r)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), argsKey{}, a)))
	})
}

// argsErrorMiddleware reports a decoding failure as the Slack error it
// corresponds to. It runs after authentication, so a call with no token gets
// not_authed whatever its body looks like.
func argsErrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a, ok := r.Context().Value(argsKey{}).(*slackArgs); ok && a.err != nil {
			slackError(w, a.err.code)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// route mounts one Web API method on GET and POST. Slack documents both verbs
// for every method.
func route(r chi.Router, method string, h http.HandlerFunc) {
	r.Get("/"+method, h)
	r.Post("/"+method, h)
}

// decodeArgs reads a call's arguments. Failures are recorded on the result and
// not returned, so the caller decides when to report them.
func decodeArgs(r *http.Request) *slackArgs {
	a := &slackArgs{values: url.Values{}, body: url.Values{}}
	query := r.URL.Query()
	if r.Method != http.MethodPost || r.Body == nil {
		mergeValues(a.values, query)
		return a
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxArgsBody+1))
	if err != nil || len(body) > maxArgsBody {
		a.err = &argError{"invalid_arguments"}
		return a
	}
	// Handlers do not read the body, but leave it readable for any that do.
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(bytes.TrimSpace(body)) == 0 {
		mergeValues(a.values, query)
		return a
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		a.err = &argError{"missing_post_type"}
		return a
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		a.err = &argError{"invalid_post_type"}
		return a
	}
	charset := strings.ToLower(params["charset"])
	if charset != "" && charset != "utf-8" && charset != "iso-8859-1" {
		a.err = &argError{"invalid_charset"}
		return a
	}

	switch mediaType {
	case "application/json":
		trimmed := bytes.TrimSpace(body)
		switch {
		case !json.Valid(trimmed):
			a.err = &argError{"invalid_json"}
		case trimmed[0] != '{':
			a.err = &argError{"json_not_object"}
		default:
			a.json = trimmed
		}
	case "application/x-www-form-urlencoded", "text/plain":
		form, err := url.ParseQuery(latin1ToUTF8(body, charset))
		if err != nil {
			a.err = &argError{"invalid_form_data"}
			return a
		}
		mergeValues(a.body, form)
		mergeValues(a.values, form)
		mergeValues(a.values, query)
	case "multipart/form-data":
		form, err := readMultipart(body, params["boundary"])
		if err != nil {
			a.err = &argError{"invalid_form_data"}
			return a
		}
		mergeValues(a.body, form)
		mergeValues(a.values, form)
		mergeValues(a.values, query)
	default:
		a.err = &argError{"invalid_post_type"}
	}
	return a
}

// echo returns the call's arguments as a flat object, for api.test. The token
// is left out so a credential is never reflected back.
func (a *slackArgs) echo() map[string]any {
	out := map[string]any{}
	if a.json != nil {
		var m map[string]any
		if json.Unmarshal(a.json, &m) == nil {
			for k, v := range m {
				out[k] = v
			}
		}
	} else {
		for k, v := range a.values {
			if len(v) > 0 {
				out[k] = v[0]
			}
		}
	}
	delete(out, "token")
	return out
}

func mergeValues(dst, src url.Values) {
	for k, v := range src {
		dst[k] = append(dst[k], v...)
	}
}

// readMultipart returns the plain fields of a multipart body. File parts are
// skipped: no Web API method in scope takes its bytes in the call itself.
func readMultipart(body []byte, boundary string) (url.Values, error) {
	if boundary == "" {
		return nil, errors.New("multipart body without a boundary")
	}
	out := url.Values{}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			continue
		}
		out.Add(part.FormName(), string(data))
	}
}

// latin1ToUTF8 converts a body declared as iso-8859-1. Other bodies are
// already UTF-8 and returned unchanged.
func latin1ToUTF8(b []byte, charset string) string {
	if charset != "iso-8859-1" {
		return string(b)
	}
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

// parseJSON fills v, a pointer to a request struct, from the call's arguments.
// The name predates the decoder: it decodes query, form, multipart and JSON
// arguments alike, by the struct's json tags.
func parseJSON(r *http.Request, v any) error {
	a, ok := r.Context().Value(argsKey{}).(*slackArgs)
	if !ok {
		a = decodeArgs(r)
	}
	if a.err != nil {
		return a.err
	}
	if a.json != nil {
		return decodeJSONArgs(a.json, v)
	}
	return decodeFormArgs(a.values, v)
}

// slackArgsError reports an error from parseJSON as the Slack error it
// corresponds to.
func slackArgsError(w http.ResponseWriter, err error) {
	var ae *argError
	if errors.As(err, &ae) {
		slackError(w, ae.code)
		return
	}
	slackError(w, "invalid_arguments")
}

// decodeJSONArgs decodes a JSON object into v. An explicit null leaves the
// field at its default, which is what Slack documents for null arguments.
func decodeJSONArgs(body []byte, v any) error {
	err := json.Unmarshal(body, v)
	if err == nil {
		return nil
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Value == "array" && typeErr.Type.Kind() == reflect.String {
			return &argError{"invalid_array_arg"}
		}
		return &argError{"invalid_arguments"}
	}
	return &argError{"invalid_json"}
}

// decodeFormArgs fills the struct v from string arguments, converting each to
// its field's type. Structured arguments (blocks, attachments, metadata) arrive
// as JSON text inside a form field and are decoded when the field takes any
// value. Fields with no argument keep their zero value.
func decodeFormArgs(values url.Values, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return &argError{"invalid_arguments"}
	}
	rv = rv.Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		vals, ok := values[name]
		if !ok || len(vals) == 0 {
			continue
		}
		if err := setFromString(rv.Field(i), vals[0]); err != nil {
			// Slack adjusts an unusable limit to something sensible and never
			// rejects it (apis/web-api/pagination).
			if name == "limit" {
				continue
			}
			return &argError{"invalid_arguments"}
		}
	}
	return nil
}

func setFromString(fv reflect.Value, s string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Bool:
		switch strings.ToLower(s) {
		case "true", "1":
			fv.SetBool(true)
		case "false", "0", "":
			fv.SetBool(false)
		default:
			return fmt.Errorf("not a boolean: %q", s)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if s == "" {
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		if s == "" {
			return nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(n)
	case reflect.Pointer:
		elem := reflect.New(fv.Type().Elem())
		if err := setFromString(elem.Elem(), s); err != nil {
			return err
		}
		fv.Set(elem)
	case reflect.Interface:
		fv.Set(reflect.ValueOf(structuredOrString(s)))
	default:
		target := reflect.New(fv.Type())
		if err := json.Unmarshal([]byte(s), target.Interface()); err != nil {
			return err
		}
		fv.Set(target.Elem())
	}
	return nil
}

// structuredOrString returns the decoded value when s is a JSON object or
// array, and s itself otherwise.
func structuredOrString(s string) any {
	t := strings.TrimSpace(s)
	if t != "" && (t[0] == '{' || t[0] == '[') && json.Valid([]byte(t)) {
		var out any
		if err := json.Unmarshal([]byte(t), &out); err == nil {
			return out
		}
	}
	return s
}
