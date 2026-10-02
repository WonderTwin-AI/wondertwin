package api

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

func ident(s string) string { return s }

func keys(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "k" + string(rune('a'+i))
	}
	return out
}

// Slack's pagination guide shows this cursor, "dXNlcjpVMEc5V0ZYTlo=", for a user
// whose id is U0G9WFXNZ. Ours is built the same way.
func TestCursorMatchesTheDocumentedExample(t *testing.T) {
	const want = "dXNlcjpVMEc5V0ZYTlo="
	if got := encodeCursor("user", "U0G9WFXNZ"); got != want {
		t.Fatalf("encodeCursor = %q, want %q", got, want)
	}
	key, ok := decodeCursor("user", want)
	if !ok || key != "U0G9WFXNZ" {
		t.Fatalf("decodeCursor = %q, %v", key, ok)
	}
}

func TestPageOfWalksTheWholeCollection(t *testing.T) {
	spec := pageSpec{kind: "k", def: 100, max: 999}
	all := keys(10)
	var seen []string
	var sizes []int
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 20 {
			t.Fatal("pagination does not terminate")
		}
		page, next, err := pageOf(all, ident, spec, cursor, 3)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, page...)
		sizes = append(sizes, len(page))
		if next == "" {
			break
		}
		cursor = next
	}
	if !slices.Equal(sizes, []int{3, 3, 3, 1}) {
		t.Errorf("page sizes = %v", sizes)
	}
	if !slices.Equal(seen, all) {
		t.Errorf("pages should add up to the collection in order, got %v", seen)
	}
}

func TestPageOfDescendingKeepsNewestFirst(t *testing.T) {
	spec := pageSpec{kind: "ts", def: 100, max: 999, desc: true}
	page, next, err := pageOf([]string{"1", "3", "5", "2", "4"}, ident, spec, "", 2)
	if err != nil || !slices.Equal(page, []string{"5", "4"}) || next == "" {
		t.Fatalf("first page = %v, %q, %v", page, next, err)
	}
	page, next, err = pageOf([]string{"1", "3", "5", "2", "4"}, ident, spec, next, 2)
	if err != nil || !slices.Equal(page, []string{"3", "2"}) || next == "" {
		t.Fatalf("second page = %v, %q, %v", page, next, err)
	}
	page, next, err = pageOf([]string{"1", "3", "5", "2", "4"}, ident, spec, next, 2)
	if err != nil || !slices.Equal(page, []string{"1"}) || next != "" {
		t.Fatalf("last page = %v, %q, %v", page, next, err)
	}
}

// A cursor points past a key, not at a position, so items arriving or leaving
// between calls neither repeat nor skip anything.
func TestPageOfIsStableWhileTheCollectionChanges(t *testing.T) {
	asc := pageSpec{kind: "k", def: 100, max: 999}
	_, next, _ := pageOf([]string{"a", "b", "c", "d", "e"}, ident, asc, "", 2)
	// b, the item the cursor came from, is deleted, and a new item arrives.
	page, _, err := pageOf([]string{"a", "bb", "c", "d", "e"}, ident, asc, next, 2)
	if err != nil || !slices.Equal(page, []string{"bb", "c"}) {
		t.Errorf("ascending, after a delete and an insert: %v, %v", page, err)
	}

	desc := pageSpec{kind: "ts", def: 100, max: 999, desc: true}
	_, next, _ = pageOf([]string{"e", "d", "c", "b", "a"}, ident, desc, "", 2)
	// A newer item arrives at the head, as a new message does.
	page, _, err = pageOf([]string{"f", "e", "d", "c", "b", "a"}, ident, desc, next, 2)
	if err != nil || !slices.Equal(page, []string{"c", "b"}) {
		t.Errorf("descending, after a newer item arrived: %v, %v", page, err)
	}
}

func TestPageOfRejectsACursorThatDoesNotCompute(t *testing.T) {
	spec := pageSpec{kind: "channel", def: 100, max: 999}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for name, cursor := range map[string]string{
		"not base64":            "!!!not-a-cursor!!!",
		"another collection":    encodeCursor("user", "U1"),
		"no kind separator":     b64("justakey"),
		"no key":                b64("channel:"),
		"base64 of gibberish":   b64("\x00\x01\x02"),
		"url-decoded plus sign": strings.ReplaceAll(encodeCursor("channel", "C1")+"x", "=", ""),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := pageOf([]string{"C1", "C2"}, ident, spec, cursor, 1); err != errInvalidCursor {
				t.Errorf("err = %v, want invalid_cursor", err)
			}
		})
	}
}

func TestPageOfAdjustsAnUnusableLimit(t *testing.T) {
	spec := pageSpec{kind: "k", def: 3, max: 5}
	all := keys(10)
	for limit, want := range map[int]int{0: 3, -4: 3, 2: 2, 5: 5, 6: 5, 100000: 5} {
		page, _, err := pageOf(all, ident, spec, "", limit)
		if err != nil || len(page) != want {
			t.Errorf("limit %d returned %d items (err %v), want %d", limit, len(page), err, want)
		}
	}
}

func TestPageOfWithNoDefaultReturnsEverything(t *testing.T) {
	spec := pageSpec{kind: "user", def: 0, max: 1000}
	page, next, err := pageOf(keys(25), ident, spec, "", 0)
	if err != nil || len(page) != 25 || next != "" {
		t.Errorf("no limit: %d items, next %q, err %v", len(page), next, err)
	}
	page, next, _ = pageOf(keys(25), ident, spec, "", 10)
	if len(page) != 10 || next == "" {
		t.Errorf("a limit still pages: %d items, next %q", len(page), next)
	}
}

func TestPageOfNeverReturnsNil(t *testing.T) {
	spec := pageSpec{kind: "k", def: 100, max: 999}
	page, next, err := pageOf([]string(nil), ident, spec, "", 0)
	if err != nil || page == nil || len(page) != 0 || next != "" {
		t.Errorf("an empty collection should page as [] with no cursor: %#v %q %v", page, next, err)
	}
}
