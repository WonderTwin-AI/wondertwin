package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/platform"
)

// jsonResult creates a ToolResult with structured JSON content.
// MCP discovery tools return JSON, not formatted text.
func jsonResult(v any) ToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return textResult(fmt.Sprintf("Error marshaling result: %v", err))
	}
	return ToolResult{
		Content: []ToolContent{{Type: "text", Text: string(data)}},
	}
}

func errorResult(msg string) ToolResult {
	return jsonResult(map[string]string{"error": msg})
}

// discoveryTools returns MCP tool entries for catalog, scan, subscribe, and request.
func discoveryTools(pc *platform.Client, cfg *config.Config) []toolEntry {
	return []toolEntry{
		{
			Tool: Tool{
				Name:        "wt_catalog",
				Description: "Search the WonderTwin twin catalog. Returns structured JSON with matching twins.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {"type": "string", "description": "Search by name or description"},
						"category": {"type": "string", "description": "Filter by category (e.g. payments, messaging)"},
						"tier": {"type": "string", "description": "Filter by tier: community or commercial"},
						"mine": {"type": "boolean", "description": "Only show org's entitled/installed twins (requires auth)"}
					}
				}`),
			},
			Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
				var p struct {
					Query    string `json:"query"`
					Category string `json:"category"`
					Tier     string `json:"tier"`
					Mine     bool   `json:"mine"`
				}
				if len(params) > 0 {
					json.Unmarshal(params, &p)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if p.Mine {
					if cfg.APIKey == "" || cfg.OrgID == "" {
						return errorResult("not authenticated: run wt login --org first")
					}
					entries, err := pc.ListOrgCatalog(ctx, cfg.OrgID, p.Category, p.Query)
					if err != nil {
						return errorResult(fmt.Sprintf("catalog query failed: %v", err))
					}
					return jsonResult(map[string]any{"twins": entries, "count": len(entries)})
				}

				entries, err := pc.ListTwins(ctx, p.Category, p.Tier, "", p.Query)
				if err != nil {
					return errorResult(fmt.Sprintf("catalog query failed: %v", err))
				}
				return jsonResult(map[string]any{"twins": entries, "count": len(entries)})
			},
		},
		{
			Tool: Tool{
				Name:        "wt_catalog_detail",
				Description: "Get detailed information about a specific twin from the catalog.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"twin_name": {"type": "string", "description": "Name of the twin to look up"}
					},
					"required": ["twin_name"]
				}`),
			},
			Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
				var p struct {
					TwinName string `json:"twin_name"`
				}
				if len(params) > 0 {
					json.Unmarshal(params, &p)
				}
				if p.TwinName == "" {
					return errorResult("twin_name is required")
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				entries, err := pc.ListTwins(ctx, "", "", "", p.TwinName)
				if err != nil {
					return errorResult(fmt.Sprintf("catalog query failed: %v", err))
				}

				// Find exact match.
				for _, e := range entries {
					if e.Name == p.TwinName {
						return jsonResult(e)
					}
				}
				return errorResult(fmt.Sprintf("twin %q not found", p.TwinName))
			},
		},
		{
			Tool: Tool{
				Name:        "wt_subscribe",
				Description: "Subscribe to a twin. The backend determines the outcome: already entitled, subscribed, trial started, or upgrade required. Returns structured JSON.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"twin_name": {"type": "string", "description": "Name of the twin to subscribe to"}
					},
					"required": ["twin_name"]
				}`),
			},
			Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
				var p struct {
					TwinName string `json:"twin_name"`
				}
				if len(params) > 0 {
					json.Unmarshal(params, &p)
				}
				if p.TwinName == "" {
					return errorResult("twin_name is required")
				}

				if cfg.APIKey == "" || cfg.OrgID == "" {
					return jsonResult(map[string]any{
						"action":     "signup_required",
						"signup_url": fmt.Sprintf("https://app.wondertwin.ai/signup?ref=mcp&twin=%s", p.TwinName),
						"message":    "Not authenticated. Sign up or run wt login --org.",
					})
				}

				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				resp, err := pc.Subscribe(ctx, cfg.OrgID, p.TwinName)
				if err != nil {
					return errorResult(fmt.Sprintf("subscribe failed: %v", err))
				}
				return jsonResult(resp)
			},
		},
		{
			Tool: Tool{
				Name:        "wt_request",
				Description: "Request a twin that doesn't exist yet. For Pro orgs, submits a structured request. Returns structured JSON.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"service_name": {"type": "string", "description": "Name of the service to request a twin for"},
						"service_url": {"type": "string", "description": "URL of the service's developer docs (optional)"},
						"category_hint": {"type": "string", "description": "Category hint for the service (optional)"}
					},
					"required": ["service_name"]
				}`),
			},
			Handler: func(_ *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
				var p struct {
					ServiceName  string `json:"service_name"`
					ServiceURL   string `json:"service_url"`
					CategoryHint string `json:"category_hint"`
				}
				if len(params) > 0 {
					json.Unmarshal(params, &p)
				}
				if p.ServiceName == "" {
					return errorResult("service_name is required")
				}

				if cfg.APIKey == "" || cfg.OrgID == "" {
					return jsonResult(map[string]any{
						"action":    "github_issue",
						"issue_url": fmt.Sprintf("https://github.com/wondertwin-ai/wondertwin/issues/new?title=Twin+Request:+%s&labels=twin-request", p.ServiceName),
						"message":   "Not authenticated. Create a GitHub issue to request this twin, or sign up for Pro.",
					})
				}

				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				result, err := pc.SubmitTwinRequest(ctx, cfg.OrgID, p.ServiceName, p.ServiceURL, p.CategoryHint)
				if err != nil {
					return errorResult(fmt.Sprintf("submit request failed: %v", err))
				}
				return jsonResult(result)
			},
		},
		{
			Tool: Tool{
				Name:        "wt_request_list",
				Description: "List your organization's twin requests and their status.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
			},
			Handler: func(_ *manifest.Manifest, _ *client.AdminClient, _ json.RawMessage) ToolResult {
				if cfg.APIKey == "" || cfg.OrgID == "" {
					return errorResult("not authenticated: run wt login --org first")
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				reqs, err := pc.ListTwinRequests(ctx, cfg.OrgID)
				if err != nil {
					return errorResult(fmt.Sprintf("list requests failed: %v", err))
				}
				return jsonResult(map[string]any{"requests": reqs, "count": len(reqs)})
			},
		},
	}
}
