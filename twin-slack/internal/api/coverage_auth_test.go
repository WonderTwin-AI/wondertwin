package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
)

// unauthenticated are the methods Slack answers without a token.
var unauthenticated = map[string]bool{"api.test": true, "oauth.v2.access": true}

// servedMethods lists every Web API method the router serves, by walking it.
func servedMethods(t *testing.T) []string {
	t.Helper()
	twin := twincore.New(&twincore.Config{Name: "twin-slack-test"})
	api.NewHandler(store.New(), twin.Middleware()).Routes(twin.Router)
	seen := map[string]bool{}
	var out []string
	err := chi.Walk(twin.Router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		name, ok := strings.CutPrefix(route, "/api/")
		if ok && method == "POST" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 70 {
		t.Fatalf("walked %d methods; the walk is broken", len(out))
	}
	return out
}

// Every method that needs a token answers not_authed without one and
// invalid_auth with a malformed one, before it reads its arguments, and
// repeats the code in x-slack-failure.
func TestEveryAuthenticatedMethodChecksTheToken(t *testing.T) {
	srv, _ := setupSlack(t)
	for _, name := range servedMethods(t) {
		if unauthenticated[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			for _, c := range []struct{ authorization, want string }{
				{"", "not_authed"},
				{"Bearer nonsense", "invalid_auth"},
			} {
				status, h, m := callAs(t, srv, "POST", "/api/"+name, formType, "channel=C1", c.authorization)
				wantError(t, status, m, c.want)
				if got := h.Get("X-Slack-Failure"); got != c.want {
					t.Errorf("authorization %q: x-slack-failure %q, want %q", c.authorization, got, c.want)
				}
			}
		})
	}
}

// A method Slack answers without a token never answers not_authed.
func TestUnauthenticatedMethodsNeedNoToken(t *testing.T) {
	srv, _ := setupSlack(t)
	for name := range unauthenticated {
		status, _, m := callAs(t, srv, "POST", "/api/"+name, formType, "", "")
		if status != http.StatusOK || m["error"] == "not_authed" || m["error"] == "invalid_auth" {
			t.Errorf("%s without a token: %d %v", name, status, m)
		}
	}
}
