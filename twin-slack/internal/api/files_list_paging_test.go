package api_test

import (
	"net/url"
	"testing"
)

// files.list pages count files at a time, default 100, with paging naming
// the page size, the total, the page and the number of pages (the files.list
// docs). A file whose upload was never completed is not listed.
func TestFilesListPages(t *testing.T) {
	srv, _ := setupSlack(t)
	var ids []string
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		id := withBytes(t, srv, name, 1)
		mustOK(t, 200, form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id + `"}]`}}))
		ids = append(ids, id)
	}
	_, pending := uploadTicket(t, srv, "pending.txt", 1)

	page := func(v url.Values) ([]string, map[string]any) {
		t.Helper()
		m := form(t, srv, "files.list", v)
		mustOK(t, 200, m)
		var got []string
		for _, f := range m["files"].([]any) {
			got = append(got, f.(map[string]any)["id"].(string))
		}
		return got, m["paging"].(map[string]any)
	}

	all, paging := page(url.Values{})
	if len(all) != 3 || paging["count"] != float64(100) || paging["total"] != float64(3) || paging["page"] != float64(1) || paging["pages"] != float64(1) {
		t.Errorf("default page: %v %v", all, paging)
	}
	for _, id := range all {
		if id == pending {
			t.Errorf("an upload that was never completed is listed: %v", all)
		}
	}

	second, paging := page(url.Values{"count": {"2"}, "page": {"2"}})
	if len(second) != 1 || second[0] != ids[2] {
		t.Errorf("page 2 of 2: %v", second)
	}
	if paging["count"] != float64(2) || paging["total"] != float64(3) || paging["page"] != float64(2) || paging["pages"] != float64(2) {
		t.Errorf("paging for page 2: %v", paging)
	}
	if beyond, _ := page(url.Values{"count": {"2"}, "page": {"5"}}); len(beyond) != 0 {
		t.Errorf("a page past the end: %v", beyond)
	}
}

// An upload whose bytes never reached the upload URL cannot be completed, and
// nothing is shared.
func TestCompleteUploadNeedsTheBytes(t *testing.T) {
	srv, _ := setupSlack(t)
	ch, _ := postIn(t, srv, "no-bytes")
	_, id := uploadTicket(t, srv, "ghost.txt", 5)
	before := len(historyTexts(t, srv, ch))

	m := form(t, srv, "files.completeUploadExternal", url.Values{"files": {`[{"id":"` + id + `"}]`}, "channel_id": {ch}})
	if m["error"] != "file_not_found" {
		t.Errorf("completing an upload with no bytes: %v", m)
	}
	if after := len(historyTexts(t, srv, ch)); after != before {
		t.Errorf("a refused complete posted a message: %d -> %d", before, after)
	}
	if chans, _ := fileInfo(t, srv, id)["channels"].([]any); len(chans) != 0 {
		t.Errorf("a refused complete shared the file: %v", chans)
	}
}
