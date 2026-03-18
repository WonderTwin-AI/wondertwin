// Package content provides a client for the WonderTwin Content API.
package content

// ContentPayload is the top-level response from the Content API.
type ContentPayload struct {
	Twin    string          `json:"twin"`
	Version string          `json:"version"`
	Quirks  []QuirkRecord   `json:"quirks,omitempty"`
}

// QuirkRecord describes a single SDK quirk.
type QuirkRecord struct {
	ID       string `json:"id"`
	Service  string `json:"service"`
	Resource string `json:"resource"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}
