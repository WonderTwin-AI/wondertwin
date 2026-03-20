package pagination

import (
	"fmt"
	"testing"
)

type item struct {
	ID   string
	Name string
}

func getID(i item) string { return i.ID }

func testItems(n int) []item {
	items := make([]item, n)
	for i := 0; i < n; i++ {
		items[i] = item{ID: fmt.Sprintf("id_%d", i+1), Name: fmt.Sprintf("Item %d", i+1)}
	}
	return items
}

// --- Cursor-based ---

func TestCursorPaginate_FirstPage(t *testing.T) {
	items := testItems(5)
	page := CursorPaginate(items, "", 2, getID)
	if len(page.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(page.Data))
	}
	if !page.HasMore {
		t.Error("expected has_more = true")
	}
	if page.Cursor != "id_2" {
		t.Errorf("cursor = %q, want id_2", page.Cursor)
	}
	if page.Total != 5 {
		t.Errorf("total = %d, want 5", page.Total)
	}
}

func TestCursorPaginate_NextPage(t *testing.T) {
	items := testItems(5)
	page := CursorPaginate(items, "id_2", 2, getID)
	if len(page.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(page.Data))
	}
	if page.Data[0].ID != "id_3" {
		t.Errorf("first item = %q, want id_3", page.Data[0].ID)
	}
	if !page.HasMore {
		t.Error("expected has_more = true")
	}
}

func TestCursorPaginate_LastPage(t *testing.T) {
	items := testItems(5)
	page := CursorPaginate(items, "id_4", 2, getID)
	if len(page.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Error("expected has_more = false")
	}
}

func TestCursorPaginate_Empty(t *testing.T) {
	page := CursorPaginate([]item{}, "", 10, getID)
	if len(page.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Data))
	}
}

// --- Offset-based ---

func TestOffsetPaginate_FirstPage(t *testing.T) {
	items := testItems(10)
	page := OffsetPaginate(items, 1, 3)
	if len(page.Data) != 3 {
		t.Errorf("expected 3 items, got %d", len(page.Data))
	}
	if page.Data[0].ID != "id_1" {
		t.Errorf("first = %q, want id_1", page.Data[0].ID)
	}
	if page.TotalCount != 10 {
		t.Errorf("total = %d, want 10", page.TotalCount)
	}
}

func TestOffsetPaginate_MiddlePage(t *testing.T) {
	items := testItems(10)
	page := OffsetPaginate(items, 4, 3)
	if len(page.Data) != 3 {
		t.Errorf("expected 3 items, got %d", len(page.Data))
	}
	if page.Data[0].ID != "id_4" {
		t.Errorf("first = %q, want id_4", page.Data[0].ID)
	}
}

func TestOffsetPaginate_BeyondEnd(t *testing.T) {
	items := testItems(5)
	page := OffsetPaginate(items, 10, 3)
	if len(page.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Data))
	}
}

// --- Page-number ---

func TestPagePaginate_FirstPage(t *testing.T) {
	items := testItems(7)
	page := PagePaginate(items, 1, 3)
	if len(page.Data) != 3 {
		t.Errorf("expected 3 items, got %d", len(page.Data))
	}
	if page.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", page.TotalPages)
	}
	if page.TotalCount != 7 {
		t.Errorf("total_count = %d, want 7", page.TotalCount)
	}
}

func TestPagePaginate_LastPage(t *testing.T) {
	items := testItems(7)
	page := PagePaginate(items, 3, 3)
	if len(page.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(page.Data))
	}
}

func TestPagePaginate_BeyondLastPage(t *testing.T) {
	items := testItems(7)
	page := PagePaginate(items, 10, 3)
	if len(page.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Data))
	}
}

func TestPagePaginate_Empty(t *testing.T) {
	page := PagePaginate([]item{}, 1, 10)
	if len(page.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Data))
	}
	if page.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", page.TotalPages)
	}
}
