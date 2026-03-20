// Package soap provides SOAP envelope handling for WonderTwin twins that
// simulate legacy SOAP/XML APIs. It parses incoming SOAP requests, extracts
// the action and body, and formats SOAP responses and faults.
package soap

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Envelope is a SOAP envelope.
type Envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Header  *Header  `xml:"Header,omitempty"`
	Body    Body     `xml:"Body"`
}

// Header is the SOAP header element.
type Header struct {
	Content []byte `xml:",innerxml"`
}

// Body is the SOAP body element.
type Body struct {
	Content []byte `xml:",innerxml"`
	Fault   *Fault `xml:"Fault,omitempty"`
}

// Fault is a SOAP fault element.
type Fault struct {
	XMLName xml.Name `xml:"Fault"`
	Code    string   `xml:"faultcode"`
	String  string   `xml:"faultstring"`
	Detail  string   `xml:"detail,omitempty"`
}

// Request represents a parsed SOAP request.
type Request struct {
	Action string // SOAPAction header value
	Body   []byte // raw XML body content
	Header []byte // raw XML header content (if present)
}

// ActionFunc handles a SOAP action. Receives the raw XML body content,
// returns the XML body content for the response.
type ActionFunc func(body []byte) ([]byte, error)

// Handler provides SOAP request routing by SOAPAction.
type Handler struct {
	actions  map[string]ActionFunc
	fallback ActionFunc
}

// NewHandler creates a new SOAP handler.
func NewHandler() *Handler {
	return &Handler{
		actions: make(map[string]ActionFunc),
	}
}

// Action registers a handler for a SOAPAction value.
func (h *Handler) Action(action string, fn ActionFunc) {
	h.actions[action] = fn
}

// Fallback sets a handler for unrecognized actions.
func (h *Handler) Fallback(fn ActionFunc) {
	h.fallback = fn
}

// ServeHTTP implements http.Handler for SOAP requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	// Parse the SOAP request.
	req, err := ParseRequest(r)
	if err != nil {
		WriteFault(w, http.StatusBadRequest, "Client", "Invalid SOAP request: "+err.Error(), "")
		return
	}

	// Find the action handler.
	fn, ok := h.actions[req.Action]
	if !ok && h.fallback != nil {
		fn = h.fallback
	}
	if fn == nil {
		WriteFault(w, http.StatusInternalServerError, "Server", "Unknown SOAPAction: "+req.Action, "")
		return
	}

	// Execute the action.
	responseBody, err := fn(req.Body)
	if err != nil {
		WriteFault(w, http.StatusInternalServerError, "Server", err.Error(), "")
		return
	}

	WriteResponse(w, http.StatusOK, responseBody)
}

// ParseRequest parses a SOAP request from an HTTP request.
func ParseRequest(r *http.Request) (*Request, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var env Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse SOAP envelope: %w", err)
	}

	action := r.Header.Get("SOAPAction")
	action = strings.Trim(action, `"`)

	req := &Request{
		Action: action,
		Body:   env.Body.Content,
	}
	if env.Header != nil {
		req.Header = env.Header.Content
	}

	return req, nil
}

// WriteResponse writes a SOAP response with the given body content.
func WriteResponse(w http.ResponseWriter, status int, bodyContent []byte) {
	env := Envelope{
		Body: Body{Content: bodyContent},
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(env)
}

// WriteFault writes a SOAP fault response.
func WriteFault(w http.ResponseWriter, status int, code, message, detail string) {
	fault := Fault{
		Code:   code,
		String: message,
		Detail: detail,
	}

	faultBytes, _ := xml.Marshal(fault)

	env := Envelope{
		Body: Body{Content: faultBytes},
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(env)
}

// MarshalBody marshals a Go value to XML for use as a SOAP body element.
func MarshalBody(v any) ([]byte, error) {
	return xml.Marshal(v)
}

// UnmarshalBody unmarshals the SOAP body XML into a Go value.
func UnmarshalBody(body []byte, v any) error {
	return xml.Unmarshal(body, v)
}
