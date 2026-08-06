// Package grpc provides gRPC service scaffolding for WonderTwin twins.
// It does NOT import the google.golang.org/grpc package — twins that need
// gRPC bring their own dependency. This package provides shared patterns:
// service registration, unary/stream handler types, error mapping, and
// a reflection-like service discovery endpoint via REST.
//
// For twins that simulate gRPC APIs, the typical pattern is:
//  1. Generate Go code from .proto files (protoc-gen-go, protoc-gen-go-grpc)
//  2. Implement the generated server interface using twinkit engines
//  3. Use this package for service metadata and REST-based discovery
//
// Many gRPC APIs also offer REST/JSON transcoding (gRPC-Gateway). For these,
// the twin can expose a chi REST handler that delegates to the same engine,
// without needing actual gRPC at all. This package supports that pattern.
package grpc

import (
	"encoding/json"
	"net/http"
	"sync"
)

// ServiceInfo describes a gRPC service for discovery/reflection.
type ServiceInfo struct {
	Name    string       `json:"name"`
	Methods []MethodInfo `json:"methods"`
}

// MethodInfo describes a single gRPC method.
type MethodInfo struct {
	Name            string `json:"name"`
	InputType       string `json:"input_type"`
	OutputType      string `json:"output_type"`
	ClientStreaming bool   `json:"client_streaming,omitempty"`
	ServerStreaming bool   `json:"server_streaming,omitempty"`
}

// Registry tracks registered gRPC services for discovery.
// Twins register their services here; the REST reflection endpoint
// exposes them for tooling like grpcurl (via JSON).
type Registry struct {
	mu       sync.RWMutex
	services map[string]*ServiceInfo
}

// NewRegistry creates a new service registry.
func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*ServiceInfo),
	}
}

// Register adds a service to the registry.
func (r *Registry) Register(svc ServiceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[svc.Name] = &svc
}

// List returns all registered services.
func (r *Registry) List() []ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ServiceInfo, 0, len(r.services))
	for _, svc := range r.services {
		result = append(result, *svc)
	}
	return result
}

// Get returns a service by name.
func (r *Registry) Get(name string) (*ServiceInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, ok := r.services[name]
	if !ok {
		return nil, false
	}
	return svc, true
}

// ReflectionHandler returns an HTTP handler that serves service metadata
// as JSON. This is a REST alternative to gRPC server reflection, useful
// for discovery tooling and documentation.
//
// GET /grpc/services — list all services
// GET /grpc/services/{name} — get service details
func (r *Registry) ReflectionHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/grpc/services", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"services": r.List(),
		})
	})

	mux.HandleFunc("/grpc/services/", func(w http.ResponseWriter, req *http.Request) {
		name := req.URL.Path[len("/grpc/services/"):]
		svc, ok := r.Get(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "service not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(svc)
	})

	return mux
}

// ErrorCode represents a gRPC status code.
type ErrorCode int

const (
	OK                 ErrorCode = 0
	Cancelled          ErrorCode = 1
	Unknown            ErrorCode = 2
	InvalidArgument    ErrorCode = 3
	DeadlineExceeded   ErrorCode = 4
	NotFound           ErrorCode = 5
	AlreadyExists      ErrorCode = 6
	PermissionDenied   ErrorCode = 7
	ResourceExhausted  ErrorCode = 8
	FailedPrecondition ErrorCode = 9
	Aborted            ErrorCode = 10
	OutOfRange         ErrorCode = 11
	Unimplemented      ErrorCode = 12
	Internal           ErrorCode = 13
	Unavailable        ErrorCode = 14
	DataLoss           ErrorCode = 15
	Unauthenticated    ErrorCode = 16
)

// StatusError represents a gRPC-style error with a code and message.
// Twins can use this in REST transcoding to return appropriate HTTP status codes.
type StatusError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *StatusError) Error() string {
	return e.Message
}

// HTTPStatus maps a gRPC error code to the conventional HTTP status code.
func (e *StatusError) HTTPStatus() int {
	switch e.Code {
	case OK:
		return http.StatusOK
	case InvalidArgument:
		return http.StatusBadRequest
	case NotFound:
		return http.StatusNotFound
	case AlreadyExists:
		return http.StatusConflict
	case PermissionDenied:
		return http.StatusForbidden
	case Unauthenticated:
		return http.StatusUnauthorized
	case ResourceExhausted:
		return http.StatusTooManyRequests
	case Unimplemented:
		return http.StatusNotImplemented
	case Unavailable:
		return http.StatusServiceUnavailable
	case DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// NewError creates a StatusError.
func NewError(code ErrorCode, message string) *StatusError {
	return &StatusError{Code: code, Message: message}
}

// WriteError writes a gRPC-style error as a JSON response with the
// appropriate HTTP status code. Useful for REST transcoding.
func WriteError(w http.ResponseWriter, err *StatusError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.HTTPStatus())
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    int(err.Code),
			"message": err.Message,
			"status":  errorCodeName(err.Code),
		},
	})
}

func errorCodeName(code ErrorCode) string {
	names := map[ErrorCode]string{
		OK: "OK", Cancelled: "CANCELLED", Unknown: "UNKNOWN",
		InvalidArgument: "INVALID_ARGUMENT", DeadlineExceeded: "DEADLINE_EXCEEDED",
		NotFound: "NOT_FOUND", AlreadyExists: "ALREADY_EXISTS",
		PermissionDenied: "PERMISSION_DENIED", ResourceExhausted: "RESOURCE_EXHAUSTED",
		FailedPrecondition: "FAILED_PRECONDITION", Aborted: "ABORTED",
		OutOfRange: "OUT_OF_RANGE", Unimplemented: "UNIMPLEMENTED",
		Internal: "INTERNAL", Unavailable: "UNAVAILABLE",
		DataLoss: "DATA_LOSS", Unauthenticated: "UNAUTHENTICATED",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return "UNKNOWN"
}
