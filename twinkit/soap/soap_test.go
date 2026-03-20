package soap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func soapRequest(action, body string) *http.Request {
	envelope := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>` + body + `</soap:Body>
</soap:Envelope>`

	req := httptest.NewRequest("POST", "/service", strings.NewReader(envelope))
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("SOAPAction", `"`+action+`"`)
	return req
}

func TestHandler_RoutesAction(t *testing.T) {
	h := NewHandler()
	h.Action("GetUser", func(body []byte) ([]byte, error) {
		return []byte(`<GetUserResponse><Name>Alice</Name></GetUserResponse>`), nil
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, soapRequest("GetUser", `<GetUser><ID>1</ID></GetUser>`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Alice") {
		t.Errorf("response should contain Alice: %s", w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/xml") {
		t.Error("Content-Type should be text/xml")
	}
}

func TestHandler_UnknownAction(t *testing.T) {
	h := NewHandler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, soapRequest("UnknownAction", `<Foo/>`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UnknownAction") {
		t.Error("fault should mention the unknown action")
	}
}

func TestHandler_Fallback(t *testing.T) {
	h := NewHandler()
	h.Fallback(func(body []byte) ([]byte, error) {
		return []byte(`<FallbackResponse/>`), nil
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, soapRequest("Anything", `<Whatever/>`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandler_ActionError(t *testing.T) {
	h := NewHandler()
	h.Action("Fail", func(body []byte) ([]byte, error) {
		return nil, &httpError{message: "something broke"}
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, soapRequest("Fail", `<Fail/>`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "something broke") {
		t.Error("fault should contain error message")
	}
}

type httpError struct{ message string }

func (e *httpError) Error() string { return e.message }

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/service", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestParseRequest(t *testing.T) {
	req := soapRequest("MyAction", `<MyRequest><Value>42</Value></MyRequest>`)
	parsed, err := ParseRequest(req)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Action != "MyAction" {
		t.Errorf("action = %q, want MyAction", parsed.Action)
	}
	if !strings.Contains(string(parsed.Body), "42") {
		t.Error("body should contain the request content")
	}
}

func TestWriteFault(t *testing.T) {
	w := httptest.NewRecorder()
	WriteFault(w, http.StatusBadRequest, "Client", "Bad input", "details here")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Bad input") {
		t.Error("fault should contain error message")
	}
	if !strings.Contains(body, "Envelope") {
		t.Error("response should be a SOAP envelope")
	}
}

func TestMarshalUnmarshalBody(t *testing.T) {
	type User struct {
		Name string `xml:"Name"`
		Age  int    `xml:"Age"`
	}

	data, err := MarshalBody(User{Name: "Bob", Age: 30})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got User
	if err := UnmarshalBody(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != "Bob" || got.Age != 30 {
		t.Errorf("got %+v", got)
	}
}
