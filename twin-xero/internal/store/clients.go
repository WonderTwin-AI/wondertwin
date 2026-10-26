package store

// Client is an app registered with the emulator through the admin plane. It is
// what strict_auth checks the token endpoint's caller against; by default any
// client id and secret is accepted and none need be registered.
type Client struct {
	Secret       string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
}

// AuthCode is an authorization code issued through the admin plane, standing in
// for the consent screen the emulator does not have. It is single use and bound
// to the client and redirect URI it was issued for.
type AuthCode struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
	Scope       string `json:"scope"`
	AuthEventID string `json:"authentication_event_id"`
	Used        bool   `json:"used"`
}
