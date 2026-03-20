package grpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistry_RegisterAndList(t *testing.T) {
	r := NewRegistry()
	r.Register(ServiceInfo{
		Name: "example.v1.UserService",
		Methods: []MethodInfo{
			{Name: "GetUser", InputType: "GetUserRequest", OutputType: "User"},
			{Name: "ListUsers", InputType: "ListUsersRequest", OutputType: "ListUsersResponse", ServerStreaming: true},
		},
	})

	services := r.List()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Name != "example.v1.UserService" {
		t.Errorf("name = %q", services[0].Name)
	}
	if len(services[0].Methods) != 2 {
		t.Errorf("methods = %d, want 2", len(services[0].Methods))
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.Register(ServiceInfo{Name: "svc.Foo", Methods: []MethodInfo{{Name: "Bar"}}})

	svc, ok := r.Get("svc.Foo")
	if !ok || svc == nil {
		t.Fatal("expected to find service")
	}

	_, ok = r.Get("svc.NotExist")
	if ok {
		t.Error("should not find nonexistent service")
	}
}

func TestReflectionHandler_ListServices(t *testing.T) {
	r := NewRegistry()
	r.Register(ServiceInfo{Name: "svc.A"})
	r.Register(ServiceInfo{Name: "svc.B"})

	handler := r.ReflectionHandler()
	req := httptest.NewRequest("GET", "/grpc/services", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	services := resp["services"].([]any)
	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}
}

func TestReflectionHandler_GetService(t *testing.T) {
	r := NewRegistry()
	r.Register(ServiceInfo{
		Name:    "svc.Users",
		Methods: []MethodInfo{{Name: "Get", InputType: "GetReq", OutputType: "User"}},
	})

	handler := r.ReflectionHandler()
	req := httptest.NewRequest("GET", "/grpc/services/svc.Users", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var svc ServiceInfo
	json.Unmarshal(w.Body.Bytes(), &svc)
	if svc.Name != "svc.Users" {
		t.Errorf("name = %q", svc.Name)
	}
}

func TestReflectionHandler_NotFound(t *testing.T) {
	r := NewRegistry()
	handler := r.ReflectionHandler()

	req := httptest.NewRequest("GET", "/grpc/services/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStatusError_HTTPMapping(t *testing.T) {
	tests := []struct {
		code   ErrorCode
		status int
	}{
		{OK, 200},
		{NotFound, 404},
		{InvalidArgument, 400},
		{PermissionDenied, 403},
		{Unauthenticated, 401},
		{Unimplemented, 501},
		{Unavailable, 503},
		{Internal, 500},
		{ResourceExhausted, 429},
		{AlreadyExists, 409},
	}

	for _, tt := range tests {
		err := NewError(tt.code, "test")
		if err.HTTPStatus() != tt.status {
			t.Errorf("code %d: HTTP status = %d, want %d", tt.code, err.HTTPStatus(), tt.status)
		}
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, NewError(NotFound, "user not found"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["status"] != "NOT_FOUND" {
		t.Errorf("status = %v, want NOT_FOUND", errObj["status"])
	}
	if errObj["message"] != "user not found" {
		t.Errorf("message = %v", errObj["message"])
	}
}
