package store

import "time"

// LogoRequest records a request for a logo.
type LogoRequest struct {
	Domain    string    `json:"domain"`
	Size      int       `json:"size,omitempty"`
	Format    string    `json:"format,omitempty"`
	Greyscale bool      `json:"greyscale,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// CustomLogo holds logo content with its media type.
type CustomLogo struct {
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}
