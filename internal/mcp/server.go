package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

// Server is an MCP server that exposes twin management tools over JSON-RPC 2.0 on stdio.
type Server struct {
	manifest *manifest.Manifest
	client   *client.AdminClient
	tools    []toolEntry
	stdin    io.Reader
	stdout   io.Writer
}

// NewServer creates a new MCP server for the given manifest.
func NewServer(m *manifest.Manifest) *Server {
	return &Server{
		manifest: m,
		client:   client.New(),
		tools:    allTools(),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
	}
}

// Serve reads JSON-RPC messages from stdin line-by-line and writes responses to stdout.
// It blocks until stdin is closed or an unrecoverable error occurs.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.stdin)
	// Allow up to 1MB per line for large tool results
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeResponse(newErrorResponse(nil, ErrCodeParse, "parse error: "+err.Error()))
			continue
		}

		resp, shouldReply := s.dispatch(&req)
		if shouldReply {
			s.writeResponse(resp)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	return nil
}

// dispatch routes a JSON-RPC request to the appropriate handler.
// Returns the response and whether a response should be sent (false for notifications).
func (s *Server) dispatch(req *Request) (Response, bool) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req), true

	case "notifications/initialized":
		// Notification — no response
		return Response{}, false

	case "tools/list":
		return s.handleToolsList(req), true

	case "tools/call":
		return s.handleToolsCall(req), true

	default:
		if req.IsNotification() {
			// Unknown notifications are silently ignored per spec
			return Response{}, false
		}
		return newErrorResponse(req.ID, ErrCodeNoMethod, "method not found: "+req.Method), true
	}
}

// handleInitialize responds to the MCP initialize handshake.
func (s *Server) handleInitialize(req *Request) Response {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "wondertwin-mcp",
			"version": "0.1.0",
		},
	}
	return newResponse(req.ID, result)
}

// handleToolsList returns the list of available MCP tools.
func (s *Server) handleToolsList(req *Request) Response {
	tools := make([]Tool, len(s.tools))
	for i, t := range s.tools {
		tools[i] = t.Tool
	}
	result := map[string]any{
		"tools": tools,
	}
	return newResponse(req.ID, result)
}

// toolsCallParams holds the parameters for a tools/call request.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// handleToolsCall dispatches a tool invocation and returns the result.
func (s *Server) handleToolsCall(req *Request) Response {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, ErrCodeInvalidParams, "invalid params: "+err.Error())
	}

	for _, t := range s.tools {
		if t.Tool.Name == params.Name {
			result := t.Handler(s.manifest, s.client, params.Arguments)
			return newResponse(req.ID, result)
		}
	}

	return newErrorResponse(req.ID, ErrCodeNoMethod, "unknown tool: "+params.Name)
}

// writeResponse marshals a Response to JSON and writes it as a single line to stdout.
func (s *Server) writeResponse(resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		// Last resort: write a hard-coded error
		fmt.Fprintf(s.stdout, `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal marshal error"}}`+"\n")
		return
	}
	fmt.Fprintf(s.stdout, "%s\n", data)
}
