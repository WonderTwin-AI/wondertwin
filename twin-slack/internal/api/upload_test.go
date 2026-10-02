package api_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/api"
)

// uploadTicket asks for an upload URL and returns it with the file id.
func uploadTicket(t *testing.T, srv *httptest.Server, filename string, length int) (string, string) {
	t.Helper()
	status, m := call(t, srv, "POST", "/api/files.getUploadURLExternal", formType,
		url.Values{"filename": {filename}, "length": {strconv.Itoa(length)}}.Encode(), true)
	mustOK(t, status, m)
	return m["upload_url"].(string), m["file_id"].(string)
}

func postBytes(t *testing.T, uploadURL, contentType string, body []byte) int {
	t.Helper()
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func fileInfo(t *testing.T, srv *httptest.Server, id string) map[string]any {
	t.Helper()
	status, m := call(t, srv, "GET", "/api/files.info?file="+id, "", "", true)
	mustOK(t, status, m)
	return m["file"].(map[string]any)
}

func TestUploadURLPointsAtTheEmulator(t *testing.T) {
	srv, _ := setupSlack(t)
	u, id := uploadTicket(t, srv, "a.txt", 3)
	if want := srv.URL + "/upload/v1/" + id; u != want {
		t.Fatalf("upload_url = %q, want %q", u, want)
	}

	// Behind a proxy such as the LocalStack gateway, the URL follows the
	// forwarded host and scheme.
	req, _ := http.NewRequest("POST", srv.URL+"/api/files.getUploadURLExternal", strings.NewReader("filename=b.txt&length=1"))
	req.Header.Set("Content-Type", formType)
	req.Header.Set("Authorization", "Bearer xoxb-test-token")
	req.Header.Set("X-Forwarded-Host", "slack.localhost.localstack.cloud:4566")
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"upload_url":"https://slack.localhost.localstack.cloud:4566/upload/v1/`) {
		t.Errorf("forwarded upload_url not used: %s", raw)
	}
}

func TestUploadStoresRawBytes(t *testing.T) {
	srv, _ := setupSlack(t)
	content := []byte("hello, file\n")
	u, id := uploadTicket(t, srv, "hello.txt", len(content))
	if code := postBytes(t, u, "application/octet-stream", content); code != 200 {
		t.Fatalf("upload status = %d, want 200", code)
	}
	if size := fileInfo(t, srv, id)["size"]; size != float64(len(content)) {
		t.Errorf("size = %v, want %d", size, len(content))
	}
}

func TestUploadStoresAMultipartFile(t *testing.T) {
	srv, _ := setupSlack(t)
	content := []byte("multipart bytes")
	u, id := uploadTicket(t, srv, "m.txt", len(content))

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("filename", "m.txt")
	part, _ := mw.CreateFormFile("file", "m.txt")
	_, _ = part.Write(content)
	_ = mw.Close()

	if code := postBytes(t, u, mw.FormDataContentType(), body.Bytes()); code != 200 {
		t.Fatalf("upload status = %d, want 200", code)
	}
	if size := fileInfo(t, srv, id)["size"]; size != float64(len(content)) {
		t.Errorf("size = %v, want %d (the file part, not the whole form)", size, len(content))
	}
}

func TestUploadWithoutAFilePartFails(t *testing.T) {
	srv, _ := setupSlack(t)
	u, id := uploadTicket(t, srv, "n.txt", 1)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("filename", "not the file")
	_ = mw.Close()

	if code := postBytes(t, u, mw.FormDataContentType(), body.Bytes()); code != http.StatusBadRequest {
		t.Fatalf("upload status = %d, want 400", code)
	}
	if size := fileInfo(t, srv, id)["size"]; size != float64(1) {
		t.Errorf("size = %v, want the announced 1 (nothing stored)", size)
	}
}

func TestUploadOverTheLimitFails(t *testing.T) {
	api.SetMaxUploadBytes(t, 4)
	srv, _ := setupSlack(t)
	u, id := uploadTicket(t, srv, "big.txt", 1)
	if code := postBytes(t, u, "application/octet-stream", []byte("0123456789")); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload status = %d, want 413", code)
	}
	if size := fileInfo(t, srv, id)["size"]; size != float64(1) {
		t.Errorf("size = %v, want the announced 1 (nothing stored)", size)
	}
}

// The running server caps every request body before the handler sees it; a
// body cut off there is still too large, not unreadable.
func TestUploadOverTheServerBodyCapFails(t *testing.T) {
	srv, _ := setupSlack(t)
	capped := httptest.NewServer(http.MaxBytesHandler(srv.Config.Handler, 4))
	t.Cleanup(capped.Close)
	u, id := uploadTicket(t, srv, "big.txt", 1)
	u = capped.URL + strings.TrimPrefix(u, srv.URL)
	if code := postBytes(t, u, "application/octet-stream", []byte("0123456789")); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload status = %d, want 413", code)
	}
	if size := fileInfo(t, srv, id)["size"]; size != float64(1) {
		t.Errorf("size = %v, want the announced 1 (nothing stored)", size)
	}
}

func TestUploadToAnUnknownTicketFails(t *testing.T) {
	srv, _ := setupSlack(t)
	if code := postBytes(t, srv.URL+"/upload/v1/F_NOPE", "", []byte("x")); code == 200 {
		t.Fatal("an upload with no ticket must not answer 200")
	}
}

func TestCompleteUploadSharesToTheChannel(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	other := seedChannel(tc, "random")
	u, id := uploadTicket(t, srv, "r.txt", 1)
	postBytes(t, u, "", []byte("r"))

	// The SDKs send files as a JSON string in a form body.
	form := url.Values{"files": {`[{"id":"` + id + `","title":"Report"}]`}, "channel_id": {ch}, "channels": {other}}
	status, m := call(t, srv, "POST", "/api/files.completeUploadExternal", formType, form.Encode(), true)
	mustOK(t, status, m)
	files := m["files"].([]any)
	got := files[0].(map[string]any)
	if got["id"] != id || got["title"] != "Report" || len(got) != 2 {
		t.Errorf("the documented answer is {id, title} per file, got %v", got)
	}

	info := fileInfo(t, srv, id)
	chans := info["channels"].([]any)
	if len(chans) != 2 || chans[0] != ch || chans[1] != other {
		t.Errorf("channels = %v, want [%s %s]", chans, ch, other)
	}
	if info["title"] != "Report" {
		t.Errorf("title = %v", info["title"])
	}
	if _, leaked := info["content"]; leaked {
		t.Error("uploaded bytes must never appear in files.info")
	}

	// Completing again does not share it twice.
	call(t, srv, "POST", "/api/files.completeUploadExternal", formType, form.Encode(), true)
	if n := len(fileInfo(t, srv, id)["channels"].([]any)); n != 2 {
		t.Errorf("a repeated complete duplicated the share: %d channels", n)
	}
}

func TestCompleteUploadErrors(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	_, id := uploadTicket(t, srv, "e.txt", 1)
	files := `[{"id":"` + id + `"}]`

	for _, c := range []struct {
		name string
		form url.Values
		want string
	}{
		{"unknown channel_id", url.Values{"files": {files}, "channel_id": {"C_NOPE"}}, "channel_not_found"},
		{"unknown entry in channels", url.Values{"files": {files}, "channels": {ch + ",C_NOPE"}}, "invalid_channel"},
		{"unknown file", url.Values{"files": {`[{"id":"F_NOPE"}]`}, "channel_id": {ch}}, "file_not_found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, m := call(t, srv, "POST", "/api/files.completeUploadExternal", formType, c.form.Encode(), true)
			wantError(t, status, m, c.want)
		})
	}
}

func TestCompleteUploadWithAnUnknownFileSharesNothing(t *testing.T) {
	srv, tc := setupSlack(t)
	ch := seedChannel(tc, "general")
	_, id := uploadTicket(t, srv, "v.txt", 1)
	form := url.Values{"files": {`[{"id":"` + id + `","title":"Changed"},{"id":"F_NOPE"}]`}, "channel_id": {ch}}
	status, m := call(t, srv, "POST", "/api/files.completeUploadExternal", formType, form.Encode(), true)
	wantError(t, status, m, "file_not_found")

	info := fileInfo(t, srv, id)
	if chans, _ := info["channels"].([]any); len(chans) != 0 {
		t.Errorf("a failed complete shared the valid file: channels = %v", chans)
	}
	if info["title"] != "v.txt" {
		t.Errorf("a failed complete changed the title: %v", info["title"])
	}
}

func TestCompleteUploadWithoutAChannelLeavesTheFilePrivate(t *testing.T) {
	srv, _ := setupSlack(t)
	_, id := uploadTicket(t, srv, "p.txt", 1)
	status, m := call(t, srv, "POST", "/api/files.completeUploadExternal", formType,
		url.Values{"files": {`[{"id":"` + id + `"}]`}}.Encode(), true)
	mustOK(t, status, m)
	if info := fileInfo(t, srv, id); info["is_public"] != false {
		t.Errorf("a file shared nowhere is private, got is_public=%v", info["is_public"])
	}
}
