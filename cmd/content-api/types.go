package main

// ContentPayload is the JSON response for GET /v1/content/{spec}.
type ContentPayload struct {
	Twin    string        `json:"twin"`
	Version string        `json:"version"`
	Quirks  []QuirkRecord `json:"quirks,omitempty"`
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
