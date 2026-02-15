package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/client"
	"github.com/wondertwin-ai/wondertwin/internal/manifest"
	"github.com/wondertwin-ai/wondertwin/internal/procmgr"
)

// Tool describes an MCP tool definition.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// ToolResult is returned from tool invocations.
type ToolResult struct {
	Content []ToolContent `json:"content"`
}

// ToolContent holds a single piece of tool output.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// textResult creates a ToolResult with a single text content item.
func textResult(text string) ToolResult {
	return ToolResult{
		Content: []ToolContent{{Type: "text", Text: text}},
	}
}

// toolHandler is a function that handles an MCP tool call.
type toolHandler func(m *manifest.Manifest, ac *client.AdminClient, params json.RawMessage) ToolResult

// toolEntry bundles a tool definition with its handler.
type toolEntry struct {
	Tool    Tool
	Handler toolHandler
}

// allTools returns the set of MCP tools the server exposes.
func allTools() []toolEntry {
	return []toolEntry{
		{
			Tool: Tool{
				Name:        "wt_up",
				Description: "Start all twins defined in wondertwin.yaml. Launches each twin binary as a background process and waits for health checks.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {}, "required": []}`),
			},
			Handler: handleUp,
		},
		{
			Tool: Tool{
				Name:        "wt_down",
				Description: "Stop all running twins. Sends SIGTERM with graceful shutdown, falling back to SIGKILL after 5 seconds.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {}, "required": []}`),
			},
			Handler: handleDown,
		},
		{
			Tool: Tool{
				Name:        "wt_status",
				Description: "Health check all twins and return their status including PID, port, and health state.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {}, "required": []}`),
			},
			Handler: handleStatus,
		},
		{
			Tool: Tool{
				Name:        "wt_reset",
				Description: "Reset state on all twins or a specific twin. When a twin name is provided, only that twin is reset.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"twin": {"type": "string", "description": "Name of a specific twin to reset (optional; omit to reset all)"}}, "required": []}`),
			},
			Handler: handleReset,
		},
		{
			Tool: Tool{
				Name:        "wt_seed",
				Description: "Seed a twin with fixture data by POSTing a JSON file to its /admin/state endpoint.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"twin": {"type": "string", "description": "Name of the twin to seed"}, "file": {"type": "string", "description": "Path to the JSON seed file"}}, "required": ["twin", "file"]}`),
			},
			Handler: handleSeed,
		},
		{
			Tool: Tool{
				Name:        "wt_inspect",
				Description: "Get the current internal state of a twin by querying its /admin/state endpoint.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"twin": {"type": "string", "description": "Name of the twin to inspect"}}, "required": ["twin"]}`),
			},
			Handler: handleInspect,
		},
		{
			Tool: Tool{
				Name:        "wt_config",
				Description: "Get or update the runtime configuration of a twin. Without 'updates', returns current config. With 'updates' (a JSON object), applies config changes.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"twin": {"type": "string", "description": "Name of the twin"}, "updates": {"type": "object", "description": "Key-value pairs to update (optional; omit to just read config)"}}, "required": ["twin"]}`),
			},
			Handler: handleConfig,
		},
		{
			Tool: Tool{
				Name:        "wt_quirks",
				Description: "List all quirks for a twin, or enable/disable a specific quirk. Without 'action', lists all quirks. With action='enable' or 'disable' and a quirk_id, toggles that quirk.",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"twin": {"type": "string", "description": "Name of the twin"}, "action": {"type": "string", "enum": ["enable", "disable"], "description": "Action to perform (optional; omit to list quirks)"}, "quirk_id": {"type": "string", "description": "ID of the quirk to toggle (required when action is set)"}}, "required": ["twin"]}`),
			},
			Handler: handleQuirks,
		},
	}
}

// ---------------------------------------------------------------------------
// Tool handlers
// ---------------------------------------------------------------------------

func handleUp(m *manifest.Manifest, ac *client.AdminClient, _ json.RawMessage) ToolResult {
	pids, _ := procmgr.LoadPids()

	var out strings.Builder
	names := m.TwinNames()

	for _, name := range names {
		twin := m.Twins[name]

		if entry, ok := pids[name]; ok && procmgr.IsRunning(entry.PID) {
			fmt.Fprintf(&out, "%-20s already running (pid %d)\n", name, entry.PID)
			continue
		}

		pid, err := procmgr.Start(name, twin, m.Settings.LogDir, m.Settings.Verbose)
		if err != nil {
			fmt.Fprintf(&out, "%-20s FAILED - %v\n", name, err)
			continue
		}

		pids[name] = procmgr.PidEntry{
			PID:    pid,
			Port:   twin.Port,
			Binary: twin.Binary,
		}
		fmt.Fprintf(&out, "%-20s started (pid %d, port %d)\n", name, pid, twin.Port)
	}

	if err := procmgr.SavePids(pids); err != nil {
		fmt.Fprintf(&out, "\nError saving pid state: %v\n", err)
	}

	// Brief pause for port binding
	time.Sleep(1500 * time.Millisecond)

	out.WriteString("\nHealth:\n")
	for _, name := range names {
		twin := m.Twins[name]
		ok, _ := ac.Health(twin.AdminPort)
		status := "healthy"
		if !ok {
			status = "unhealthy"
		}
		fmt.Fprintf(&out, "%-20s %s  http://localhost:%d\n", name, status, twin.Port)
	}

	return textResult(out.String())
}

func handleDown(_ *manifest.Manifest, _ *client.AdminClient, _ json.RawMessage) ToolResult {
	pids, err := procmgr.LoadPids()
	if err != nil {
		return textResult(fmt.Sprintf("Error loading pid state: %v", err))
	}

	if len(pids) == 0 {
		return textResult("No twins running.")
	}

	var out strings.Builder
	for name, entry := range pids {
		if !procmgr.IsRunning(entry.PID) {
			fmt.Fprintf(&out, "%-20s already stopped\n", name)
			continue
		}
		if err := procmgr.Stop(name, entry); err != nil {
			fmt.Fprintf(&out, "%-20s FAILED - %v\n", name, err)
		} else {
			fmt.Fprintf(&out, "%-20s stopped (was pid %d)\n", name, entry.PID)
		}
	}

	procmgr.RemovePidFile()
	out.WriteString("\nAll twins stopped.")
	return textResult(out.String())
}

func handleStatus(m *manifest.Manifest, ac *client.AdminClient, _ json.RawMessage) ToolResult {
	pids, _ := procmgr.LoadPids()

	var out strings.Builder
	fmt.Fprintf(&out, "%-20s %-8s %-7s %-11s %s\n", "TWIN", "PID", "PORT", "HEALTH", "URL")
	fmt.Fprintf(&out, "%-20s %-8s %-7s %-11s %s\n", "----", "---", "----", "------", "---")

	for _, name := range m.TwinNames() {
		twin := m.Twins[name]
		pidStr := "-"
		health := "stopped"

		if entry, ok := pids[name]; ok && procmgr.IsRunning(entry.PID) {
			pidStr = fmt.Sprintf("%d", entry.PID)
			ok, _ := ac.Health(twin.AdminPort)
			if ok {
				health = "healthy"
			} else {
				health = "unhealthy"
			}
		}

		fmt.Fprintf(&out, "%-20s %-8s %-7d %-11s http://localhost:%d\n",
			name, pidStr, twin.Port, health, twin.Port)
	}

	return textResult(out.String())
}

type resetParams struct {
	Twin string `json:"twin"`
}

func handleReset(m *manifest.Manifest, ac *client.AdminClient, params json.RawMessage) ToolResult {
	var p resetParams
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}

	pids, _ := procmgr.LoadPids()

	names := m.TwinNames()
	if p.Twin != "" {
		// Validate the twin exists
		if _, err := m.Twin(p.Twin); err != nil {
			return textResult(fmt.Sprintf("Error: %v", err))
		}
		names = []string{p.Twin}
	}

	var out strings.Builder
	for _, name := range names {
		twin := m.Twins[name]
		if entry, ok := pids[name]; !ok || !procmgr.IsRunning(entry.PID) {
			fmt.Fprintf(&out, "%-20s skipped (not running)\n", name)
			continue
		}

		resp, err := ac.Reset(twin.AdminPort)
		if err != nil {
			fmt.Fprintf(&out, "%-20s FAILED - %v\n", name, err)
		} else {
			fmt.Fprintf(&out, "%-20s reset   %s\n", name, resp)
		}
	}

	return textResult(out.String())
}

type seedParams struct {
	Twin string `json:"twin"`
	File string `json:"file"`
}

func handleSeed(m *manifest.Manifest, ac *client.AdminClient, params json.RawMessage) ToolResult {
	var p seedParams
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}

	if p.Twin == "" || p.File == "" {
		return textResult("Error: both 'twin' and 'file' arguments are required")
	}

	twin, err := m.Twin(p.Twin)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err))
	}

	resp, err := ac.Seed(twin.AdminPort, p.File)
	if err != nil {
		return textResult(fmt.Sprintf("Error seeding %s: %v", p.Twin, err))
	}

	return textResult(fmt.Sprintf("Seeded %s: %s", p.Twin, resp))
}

type inspectParams struct {
	Twin string `json:"twin"`
}

func handleInspect(m *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
	var p inspectParams
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}

	if p.Twin == "" {
		return textResult("Error: 'twin' argument is required")
	}

	twin, err := m.Twin(p.Twin)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err))
	}

	// GET /admin/state to retrieve current twin state
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/admin/state", twin.AdminPort))
	if err != nil {
		return textResult(fmt.Sprintf("Error inspecting %s: %v", p.Twin, err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return textResult(fmt.Sprintf("Error inspecting %s: status %d: %s", p.Twin, resp.StatusCode, body))
	}

	return textResult(string(body))
}

type configParams struct {
	Twin    string         `json:"twin"`
	Updates map[string]any `json:"updates"`
}

func handleConfig(m *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
	var p configParams
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}

	if p.Twin == "" {
		return textResult("Error: 'twin' argument is required")
	}

	twin, err := m.Twin(p.Twin)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err))
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	if len(p.Updates) > 0 {
		// PUT /admin/config with updates
		body, _ := json.Marshal(p.Updates)
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("http://localhost:%d/admin/config", twin.AdminPort), strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return textResult(fmt.Sprintf("Error updating config for %s: %v", p.Twin, err))
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return textResult(fmt.Sprintf("Error updating config for %s: status %d: %s", p.Twin, resp.StatusCode, respBody))
		}
		return textResult(string(respBody))
	}

	// GET /admin/config
	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/admin/config", twin.AdminPort))
	if err != nil {
		return textResult(fmt.Sprintf("Error fetching config for %s: %v", p.Twin, err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return textResult(fmt.Sprintf("Error fetching config for %s: status %d: %s", p.Twin, resp.StatusCode, body))
	}
	return textResult(string(body))
}

type quirksParams struct {
	Twin    string `json:"twin"`
	Action  string `json:"action"`
	QuirkID string `json:"quirk_id"`
}

func handleQuirks(m *manifest.Manifest, _ *client.AdminClient, params json.RawMessage) ToolResult {
	var p quirksParams
	if len(params) > 0 {
		json.Unmarshal(params, &p)
	}

	if p.Twin == "" {
		return textResult("Error: 'twin' argument is required")
	}

	twin, err := m.Twin(p.Twin)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err))
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	if p.Action != "" {
		if p.QuirkID == "" {
			return textResult("Error: 'quirk_id' is required when 'action' is specified")
		}

		var method string
		switch p.Action {
		case "enable":
			method = http.MethodPut
		case "disable":
			method = http.MethodDelete
		default:
			return textResult(fmt.Sprintf("Error: unknown action %q (use 'enable' or 'disable')", p.Action))
		}

		req, _ := http.NewRequest(method, fmt.Sprintf("http://localhost:%d/admin/quirks/%s", twin.AdminPort, p.QuirkID), nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			return textResult(fmt.Sprintf("Error toggling quirk for %s: %v", p.Twin, err))
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return textResult(fmt.Sprintf("Error toggling quirk for %s: status %d: %s", p.Twin, resp.StatusCode, body))
		}
		return textResult(string(body))
	}

	// GET /admin/quirks
	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/admin/quirks", twin.AdminPort))
	if err != nil {
		return textResult(fmt.Sprintf("Error fetching quirks for %s: %v", p.Twin, err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return textResult(fmt.Sprintf("Error fetching quirks for %s: status %d: %s", p.Twin, resp.StatusCode, body))
	}
	return textResult(string(body))
}
