package api_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func getFile(t *testing.T, link, auth string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

// A completed upload has url_private and url_private_download, in Slack's
// files-pri shape on the host the client called, and both serve the bytes to a
// caller with a token.
func TestUploadedFilesCanBeDownloaded(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "downloads")
	upload, id := uploadTicket(t, srv, "notes.txt", 5)
	if status := postBytes(t, upload, "", []byte("hello")); status != 200 {
		t.Fatalf("upload: %d", status)
	}
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id + `"}]`}, "channel_id": {ch}}))

	f := fileInfo(t, srv, id)
	private, _ := f["url_private"].(string)
	download, _ := f["url_private_download"].(string)
	if private != srv.URL+"/files-pri/T0001-"+id+"/notes.txt" || download != srv.URL+"/files-pri/T0001-"+id+"/download/notes.txt" {
		t.Fatalf("private URLs: %q %q", private, download)
	}

	status, h, body := getFile(t, private, "Bearer xoxb-reader")
	if status != 200 || body != "hello" || !strings.HasPrefix(h.Get("Content-Type"), "text/plain") || h.Get("Content-Disposition") != "" {
		t.Errorf("url_private: %d %v %q", status, h, body)
	}
	status, h, body = getFile(t, download, "Bearer xoxp-reader")
	if status != 200 || body != "hello" || !strings.Contains(h.Get("Content-Disposition"), "attachment") {
		t.Errorf("url_private_download: %d %v %q", status, h, body)
	}

	if status, _, _ := getFile(t, private, ""); status != http.StatusForbidden {
		t.Errorf("no token: %d", status)
	}
	if status, _, _ := getFile(t, private, "Bearer not-a-token"); status != http.StatusForbidden {
		t.Errorf("a malformed token: %d", status)
	}
	if status, _, _ := getFile(t, srv.URL+"/files-pri/T0001-"+id+"/other.txt", "Bearer xoxb-reader"); status != http.StatusNotFound {
		t.Errorf("wrong name: %d", status)
	}
	if status, _, _ := getFile(t, srv.URL+"/files-pri/T9999-"+id+"/notes.txt", "Bearer xoxb-reader"); status != http.StatusNotFound {
		t.Errorf("wrong team: %d", status)
	}

	// Behind a proxy, the URLs follow the host the client called, as the
	// upload URL does.
	upload2, id2 := uploadTicket(t, srv, "b.txt", 1)
	postBytes(t, upload2, "", []byte("b"))
	req, _ := http.NewRequest("POST", srv.URL+"/api/files.completeUploadExternal",
		strings.NewReader(url.Values{"files": {`[{"id":"` + id2 + `"}]`}}.Encode()))
	req.Header.Set("Content-Type", formType)
	req.Header.Set("Authorization", "Bearer xoxb-test-token")
	req.Header.Set("X-Forwarded-Host", "slack.localhost.localstack.cloud:4566")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := fileInfo(t, srv, id2)["url_private"]; got != "http://slack.localhost.localstack.cloud:4566/files-pri/T0001-"+id2+"/b.txt" {
		t.Errorf("url_private behind a proxy: %v", got)
	}
}
