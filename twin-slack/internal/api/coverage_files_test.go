package api_test

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// uploaded runs the external upload flow and returns the file id.
func uploaded(t *testing.T, srv *httptest.Server, name string) string {
	t.Helper()
	u, id := uploadTicket(t, srv, name, 1)
	postBytes(t, u, "", []byte("x"))
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id + `"}]`}}))
	return id
}

// files.sharedPublicURL makes a file public with a permalink, and
// files.revokePublicURL makes it private again.
func TestFilesPublicURL(t *testing.T) {
	srv, _ := setupSlack(t)
	id := uploaded(t, srv, "pub.txt")

	shared := form(t, srv, "files.sharedPublicURL", url.Values{"file": {id}})
	mustOK(t, 200, shared)
	f := shared["file"].(map[string]any)
	if f["id"] != id || f["is_public"] != true || f["permalink"] == "" {
		t.Errorf("sharedPublicURL answer: %v", f)
	}
	if info := fileInfo(t, srv, id); info["is_public"] != true {
		t.Errorf("files.info after sharing: %v", info)
	}

	revoked := form(t, srv, "files.revokePublicURL", url.Values{"file": {id}})
	mustOK(t, 200, revoked)
	if f := revoked["file"].(map[string]any); f["is_public"] != false {
		t.Errorf("revokePublicURL answer: %v", f)
	}
	if info := fileInfo(t, srv, id); info["is_public"] != false {
		t.Errorf("files.info after revoking: %v", info)
	}

	for _, method := range []string{"files.sharedPublicURL", "files.revokePublicURL"} {
		wantErrors(t, srv, method, map[string]errCase{
			"no file":      {url.Values{}, "file_not_found"},
			"unknown file": {url.Values{"file": {"F-nope"}}, "file_not_found"},
		})
	}
}

// files.delete removes a file once; a second delete is file_not_found.
func TestFilesDeleteErrors(t *testing.T) {
	srv, _ := setupSlack(t)
	id := uploaded(t, srv, "gone.txt")
	mustOK(t, 200, form(t, srv, "files.delete", url.Values{"file": {id}}))
	wantErrors(t, srv, "files.delete", map[string]errCase{
		"deleted twice": {url.Values{"file": {id}}, "file_not_found"},
		"no file":       {url.Values{}, "file_not_found"},
	})
	wantErrors(t, srv, "files.info", map[string]errCase{
		"deleted": {url.Values{"file": {id}}, "file_not_found"},
	})
}
