package platform

// InstallOutcome is the wire-level outcome enum returned by the
// MCP install endpoint. Mirrors the server-side enum from
// wondertwin-app/internal/billing/install_envelope.go which itself
// projects the schema at architecture/schemas/mcp-subscribe-outcome-v1.json
// in wondertwin-docs.
//
// Per adr-mcp-subscribe-outcomes §4 forward-compat rule: clients
// that encounter an unknown InstallOutcome value MUST surface
// ClientFacingMessage verbatim and treat the response as a
// synthetic policy_error — i.e. do not crash, do not retry, do not
// assume any particular semantic.
type InstallOutcome string

const (
	OutcomeInstalled               InstallOutcome = "installed"
	OutcomeAlreadyInstalledRefresh InstallOutcome = "already_installed_refreshed"
	OutcomeAlreadyEntitled         InstallOutcome = "already_entitled"
	OutcomeTrialStartedInstalled   InstallOutcome = "trial_started_and_installed"
	OutcomeQueued                  InstallOutcome = "queued"
	OutcomeDenied                  InstallOutcome = "denied"
	OutcomeUpgradeRequired         InstallOutcome = "upgrade_required"
	OutcomeSetupRequired           InstallOutcome = "setup_required"
	OutcomePolicyError             InstallOutcome = "policy_error"
)

// InstallEnvelope is the wire shape of the MCP install response.
// Mirror of wondertwin-app/internal/billing/InstallEnvelope; field
// names + JSON tags pinned by the schema.
type InstallEnvelope struct {
	Outcome             InstallOutcome `json:"outcome"`
	TwinID              string         `json:"twin_id"`
	OutcomeDetail       any            `json:"outcome_detail"`
	ClientFacingMessage string         `json:"client_facing_message"`
	SchemaVersion       int            `json:"schema_version"`
	MCPMetadata         *EnvelopeMeta  `json:"mcp_metadata,omitempty"`
}

// EnvelopeMeta is the optional per-response metadata; today carries
// only license-freshness signals. The license_refresh_required flag
// is the trigger for the wt_license_refresh path (separate PR).
type EnvelopeMeta struct {
	LicenseCurrentIssuedAt string `json:"license_current_issued_at,omitempty"`
	LicenseRefreshRequired bool   `json:"license_refresh_required,omitempty"`
}

// InstallBundle is delivered in outcomes that produce an install:
// installed, already_installed_refreshed, already_entitled (refresh
// only), trial_started_and_installed. Runtime decodes
// LicenseFileB64, downloads BinaryURL with BinarySHA256 verification,
// and atomic-commits both per adr-license-delivery-via-mcp §1.
type InstallBundle struct {
	BinaryURL       string `json:"binary_url"`
	BinarySHA256    string `json:"binary_sha256"`
	BinaryVersion   string `json:"binary_version"`
	LicenseFileB64  string `json:"license_file"`
	LicenseIssuedAt string `json:"license_issued_at"`
	LicenseNotAfter string `json:"license_not_after"`
}

// InstalledDetail is OutcomeDetail when an install bundle is present.
// JSON-decode into this when Outcome is installed, already_installed_
// refreshed, or trial_started_and_installed.
type InstalledDetail struct {
	SubscriptionID string         `json:"subscription_id"`
	TwinVersion    string         `json:"twin_version"`
	InstallBundle  *InstallBundle `json:"install_bundle"`
}

// QueuedDetail is OutcomeDetail when the install was queued. The
// agent stores QueueID and polls wt_subscribe_status (separate PR)
// until status changes.
type QueuedDetail struct {
	QueueID    string `json:"queue_id"`
	ExpiresAt  string `json:"expires_at"`
	StatusURL  string `json:"status_url"`
	ReasonCode string `json:"reason_code"`
}

// DeniedDetail is OutcomeDetail when policy denied the install.
// denied is one-way in the public API (adr-agent-subscribe-queue §3) —
// the agent must not retry; surface ClientFacingMessage to the user.
type DeniedDetail struct {
	ReasonCode     string `json:"reason_code"`
	DeniedByUserID string `json:"denied_by_user_id,omitempty"`
	DecisionNote   string `json:"decision_note,omitempty"`
}

// SetupRequiredDetail is OutcomeDetail when the org needs out-of-band
// setup before the install can proceed. SetupURL is the customer-
// portal landing page the agent surfaces to the user.
type SetupRequiredDetail struct {
	SetupURL   string `json:"setup_url"`
	ReasonCode string `json:"reason_code,omitempty"`
}

// PolicyErrorDetail is OutcomeDetail for server-side misconfiguration
// AND for the forward-compat fallback an older client constructs when
// it sees an unknown InstallOutcome value.
type PolicyErrorDetail struct {
	ErrorCode     string `json:"error_code"`
	ErrorMessage  string `json:"error_message"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// InstallRequest is the HTTP body for POST /v1/accounts/{accountID}/mcp/install.
type InstallRequest struct {
	TwinID        string `json:"twin_id"`
	PricingID     string `json:"pricing_id"`
	AgentClientID string `json:"agent_client_id,omitempty"`
}
