// Package mcp implements an MCP (Model Context Protocol) server over stdio,
// exposing WonderTwin management operations as tools for AI coding agents.
package mcp

import "encoding/json"

// Request represents a JSON-RPC 2.0 request message.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // number, string, or null; absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if this is a JSON-RPC notification (no id field).
func (r *Request) IsNotification() bool {
	return r.ID == nil || len(r.ID) == 0
}

// Response represents a JSON-RPC 2.0 response message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParse      = -32700
	ErrCodeInvalidReq = -32600
	ErrCodeNoMethod   = -32601
	ErrCodeInvalidParams = -32602
	ErrCodeInternal   = -32603
)

// newResponse creates a successful JSON-RPC response.
func newResponse(id json.RawMessage, result any) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// newErrorResponse creates an error JSON-RPC response.
func newErrorResponse(id json.RawMessage, code int, message string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}
