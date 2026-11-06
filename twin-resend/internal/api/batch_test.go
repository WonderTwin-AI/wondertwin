package api_test

import (
	"reflect"
	"testing"
)

func batchItem(over map[string]any) map[string]any {
	return baseSend(over)
}

func missingFrom() map[string]any {
	b := baseSend(nil)
	delete(b, "from")
	return b
}

func TestBatchStrict(t *testing.T) {
	_, tc := setupResend(t)
	for _, headers := range []map[string]string{
		resendHeaders,
		{"Authorization": resendHeaders["Authorization"], "x-batch-validation": "strict"},
	} {
		resp := tc.DoWithHeaders("POST", "/emails/batch", []map[string]any{batchItem(nil), missingFrom()}, headers)
		resp.AssertStatus(422)
		m := resp.JSONMap()
		if m["name"] != "validation_error" || m["message"] != "Missing `from` field." {
			t.Errorf("got %v", m)
		}
	}
	if n := len(tc.Get("/admin/emails").JSONMap()["emails"].([]any)); n != 0 {
		t.Errorf("a failed strict batch stored %d emails", n)
	}
}

func TestBatchPermissive(t *testing.T) {
	_, tc := setupResend(t)
	headers := map[string]string{"Authorization": resendHeaders["Authorization"], "x-batch-validation": "permissive"}
	resp := tc.DoWithHeaders("POST", "/emails/batch", []map[string]any{batchItem(nil), missingFrom()}, headers)
	resp.AssertStatus(202)
	m := resp.JSONMap()
	if data, _ := m["data"].([]any); len(data) != 1 {
		t.Errorf("data %v, want one id", m["data"])
	}
	want := []any{map[string]any{"index": float64(1), "message": "Missing `from` field."}}
	if !reflect.DeepEqual(m["errors"], want) {
		t.Errorf("errors %v, want %v", m["errors"], want)
	}
}

func TestBatchValidHasNoErrorsKey(t *testing.T) {
	_, tc := setupResend(t)
	resp := resendPost(tc, "/emails/batch", []map[string]any{batchItem(nil), batchItem(map[string]any{"to": "delivered+b@resend.dev"})})
	resp.AssertStatus(200)
	m := resp.JSONMap()
	if _, ok := m["errors"]; ok {
		t.Errorf("valid batch carries errors: %v", m)
	}
	if data, _ := m["data"].([]any); len(data) != 2 {
		t.Errorf("data %v, want two ids", m["data"])
	}
}

func TestBatchOver100(t *testing.T) {
	_, tc := setupResend(t)
	items := make([]map[string]any, 101)
	for i := range items {
		items[i] = batchItem(nil)
	}
	resp := resendPost(tc, "/emails/batch", items)
	resp.AssertStatus(422)
	if m := resp.JSONMap(); m["message"] != "Your batch request contains more than 100 messages." {
		t.Errorf("got %v", m)
	}
	items = items[:100]
	resendPost(tc, "/emails/batch", items).AssertStatus(200)
}

func TestBatchRefusesAttachments(t *testing.T) {
	_, tc := setupResend(t)
	item := batchItem(map[string]any{"attachments": []map[string]string{{"filename": "a.txt", "content": "aGk="}}})
	resendPost(tc, "/emails/batch", []map[string]any{item}).AssertStatus(422)
}
