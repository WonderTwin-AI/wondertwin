package api_test

import (
	"net/url"
	"strconv"
	"testing"
)

// files.getUploadURLExternal requires filename and length, and records a
// mimetype and filetype from the name.
func TestUploadTicketArgumentsAndTypes(t *testing.T) {
	srv, _ := setupSlack(t)
	wantErrors(t, srv, "files.getUploadURLExternal", map[string]errCase{
		"no filename": {url.Values{"length": {"3"}}, "invalid_arguments"},
		"no length":   {url.Values{"filename": {"a.txt"}}, "invalid_arguments"},
		"zero length": {url.Values{"filename": {"a.txt"}, "length": {"0"}}, "invalid_arguments"},
	})
	_, id := uploadTicket(t, srv, "chart.PNG", 10)
	f := fileInfo(t, srv, id)
	if f["mimetype"] != "image/png" || f["filetype"] != "png" {
		t.Errorf("types for chart.PNG: %v %v", f["mimetype"], f["filetype"])
	}
	_, id = uploadTicket(t, srv, "notes.txt", 3)
	if f := fileInfo(t, srv, id); f["mimetype"] != "text/plain" || f["filetype"] != "text" {
		t.Errorf("types for notes.txt: %v %v", f["mimetype"], f["filetype"])
	}
}

// Completing an upload with a channel posts a file_share message there,
// with the initial comment as its text and the file attached.
func TestCompleteUploadPostsAFileShareMessage(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, parent := postIn(t, srv, "shared")
	id := withBytes(t, srv, "report.pdf", 4)
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{
		"files": {`[{"id":"` + id + `","title":"Report"}]`}, "channel_id": {ch}, "initial_comment": {"Q3 numbers"},
	}))
	latest := form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)[0].(map[string]any)
	files, _ := latest["files"].([]any)
	if latest["subtype"] != "file_share" || latest["text"] != "Q3 numbers" || len(files) != 1 || files[0].(map[string]any)["id"] != id {
		t.Errorf("share message: %v", latest)
	}
	if files[0].(map[string]any)["title"] != "Report" {
		t.Errorf("shared file title: %v", files[0])
	}

	// With thread_ts the share is a reply in that thread.
	id2 := withBytes(t, srv, "b.txt", 1)
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{
		"files": {`[{"id":"` + id2 + `"}]`}, "channel_id": {ch}, "thread_ts": {parent},
	}))
	if n := len(form(t, srv, "conversations.replies", url.Values{"channel": {ch}, "ts": {parent}})["messages"].([]any)); n != 2 {
		t.Errorf("thread after a threaded share: %d messages, want 2", n)
	}

	// Completing without a channel shares nowhere and posts nothing.
	before := len(form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any))
	id3 := withBytes(t, srv, "c.txt", 1)
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id3 + `"}]`}}))
	if after := len(form(t, srv, "conversations.history", url.Values{"channel": {ch}})["messages"].([]any)); after != before {
		t.Errorf("a private upload posted a message: %d -> %d", before, after)
	}
}

// files.list filters by channel, user, types and creation time.
func TestFilesListFilters(t *testing.T) {
	srv, tc := setupSlack(t)
	ch, _ := postIn(t, srv, "files")
	share := func(name string) string {
		id := withBytes(t, srv, name, 1)
		mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id + `"}]`}, "channel_id": {ch}}))
		return id
	}
	img := share("a.png")
	created := int64(fileInfo(t, srv, img)["created"].(float64))
	tc.Post("/admin/time/advance", map[string]any{"duration": "1h"})
	pdf := share("b.pdf")
	// Completed but shared nowhere, so private.
	private := withBytes(t, srv, "c.zip", 1)
	mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + private + `"}]`}}))
	theirs := formAs(t, srv, "xoxp-uploader", "files.getUploadURLExternal", url.Values{"filename": {"d.txt"}, "length": {"1"}})
	mustOK(t, 200, theirs)
	other := theirs["file_id"].(string)
	postBytes(t, theirs["upload_url"].(string), "", []byte("d"))
	mustOK(t, 200, formAs(t, srv, "xoxp-uploader", "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + other + `"}]`}}))

	ids := func(v url.Values) []string {
		m := form(t, srv, "files.list", v)
		mustOK(t, 200, m)
		var out []string
		for _, f := range m["files"].([]any) {
			out = append(out, f.(map[string]any)["id"].(string))
		}
		return out
	}
	check := func(name string, v url.Values, want ...string) {
		t.Helper()
		got := ids(v)
		if len(got) != len(want) {
			t.Errorf("%s: %v, want %v", name, got, want)
			return
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: %v, want %v", name, got, want)
				return
			}
		}
	}
	check("all", url.Values{}, img, pdf, private, other)
	check("channel", url.Values{"channel": {ch}}, img, pdf)
	check("images", url.Values{"types": {"images"}}, img)
	check("pdfs and zips", url.Values{"types": {"pdfs,zips"}}, pdf, private)
	check("snippets", url.Values{"types": {"snippets"}})
	check("user", url.Values{"user": {"U_BOT"}}, img, pdf, private)
	check("ts_from", url.Values{"ts_from": {strconv.FormatInt(created+1, 10)}}, pdf, private, other)
	check("ts_to", url.Values{"ts_to": {strconv.FormatInt(created, 10)}}, img)

	wantErrors(t, srv, "files.list", map[string]errCase{
		"unknown type": {url.Values{"types": {"movies"}}, "unknown_type"},
		"unknown user": {url.Values{"user": {"UNOPE"}}, "user_not_found"},
	})
}
