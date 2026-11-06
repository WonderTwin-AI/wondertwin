package api

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// recipients is an address field that Resend accepts as a single string or
// an array of strings (quirk envelope-002). It is stored as an array.
type recipients []string

func (r *recipients) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		if one == "" {
			*r = nil
		} else {
			*r = recipients{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*r = many
	return nil
}

var (
	blockTag = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/h[1-6]|/li|/tr)\b[^>]*>`)
	anyTag   = regexp.MustCompile(`<[^>]*>`)
	blankRun = regexp.MustCompile(`\n{3,}`)
)

// textFor returns the plain-text body Resend stores (quirk lifecycle-008):
// the text sent, or when text is omitted, one generated from html. An empty
// text string opts out of generation.
func textFor(req sendEmailRequest) string {
	if req.Text != nil {
		return *req.Text
	}
	if req.HTML == "" {
		return ""
	}
	s := blockTag.ReplaceAllString(req.HTML, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.TrimSpace(blankRun.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
