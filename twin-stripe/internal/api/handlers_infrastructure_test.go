package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twin-stripe/internal/api"
)

func TestCreateAndGetWebhookEndpoint(t *testing.T) {
	srv, tc := setupStripe(t)

	// Without url param, should get 400
	createResp := tc.DoWithHeaders("POST", "/v1/webhook_endpoints", nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	createResp.AssertStatus(400)
	createResp.AssertBodyContains("url")

	// Create with proper form data
	formBody := "url=https://example.com/webhook&enabled_events[]=charge.succeeded"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/webhook_endpoints", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpResp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", httpResp.StatusCode)
	}

	m := decodeJSONBody(t, httpResp.Body)

	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected webhook endpoint id")
	}
	if m["object"] != "webhook_endpoint" {
		t.Errorf("expected object=webhook_endpoint, got %v", m["object"])
	}
	secret, _ := m["secret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Errorf("expected secret to start with whsec_, got %v", secret)
	}
	if m["status"] != "enabled" {
		t.Errorf("expected status=enabled, got %v", m["status"])
	}

	// Get webhook endpoint
	getResp := stripeGet(tc, "/v1/webhook_endpoints/"+id)
	getResp.AssertStatus(200)
	gm := getResp.JSONMap()
	if gm["id"] != id {
		t.Errorf("expected id=%s, got %v", id, gm["id"])
	}
	if gm["url"] != "https://example.com/webhook" {
		t.Errorf("expected url=https://example.com/webhook, got %v", gm["url"])
	}
}

func TestListAndDeleteWebhookEndpoints(t *testing.T) {
	srv, tc := setupStripe(t)

	// Create two webhook endpoints
	for _, u := range []string{"https://example.com/wh1", "https://example.com/wh2"} {
		formBody := "url=" + u
		r, _ := http.NewRequest("POST", srv.URL+"/v1/webhook_endpoints", strings.NewReader(formBody))
		r.Header.Set("Authorization", "Bearer sk_test_sim_123")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}

	// List
	listResp := stripeGet(tc, "/v1/webhook_endpoints")
	listResp.AssertStatus(200)
	lm := listResp.JSONMap()
	data, ok := lm["data"].([]any)
	if !ok || len(data) < 2 {
		t.Fatalf("expected at least 2 webhook endpoints, got %v", lm["data"])
	}

	// Delete first
	first := data[0].(map[string]any)
	firstID := first["id"].(string)
	delResp := tc.DoWithHeaders("DELETE", "/v1/webhook_endpoints/"+firstID, nil, map[string]string{
		"Authorization": "Bearer sk_test_sim_123",
	})
	delResp.AssertStatus(200)
	dm := delResp.JSONMap()
	if dm["deleted"] != true {
		t.Error("expected deleted=true")
	}

	// Verify gone
	stripeGet(tc, "/v1/webhook_endpoints/"+firstID).AssertStatus(404)
}

func TestCreateAndGetFile(t *testing.T) {
	srv, tc := setupStripe(t)

	// Create file with form-encoded (simplified, no actual multipart file upload)
	formBody := "purpose=dispute_evidence"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/files", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	m := decodeJSONBody(t, resp.Body)

	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected file id")
	}
	if m["object"] != "file" {
		t.Errorf("expected object=file, got %v", m["object"])
	}
	if m["purpose"] != "dispute_evidence" {
		t.Errorf("expected purpose=dispute_evidence, got %v", m["purpose"])
	}
	fileURL, _ := m["url"].(string)
	if !strings.Contains(fileURL, id) {
		t.Errorf("expected url to contain file id, got %v", fileURL)
	}

	// Get file
	getResp := stripeGet(tc, "/v1/files/"+id)
	getResp.AssertStatus(200)
	gm := getResp.JSONMap()
	if gm["id"] != id {
		t.Errorf("expected id=%s, got %v", id, gm["id"])
	}
}

func TestListFiles(t *testing.T) {
	srv, tc := setupStripe(t)

	// Create a file
	formBody := "purpose=account_requirement"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/files", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(r)
	resp.Body.Close()

	listResp := stripeGet(tc, "/v1/files")
	listResp.AssertStatus(200)
	lm := listResp.JSONMap()
	data, ok := lm["data"].([]any)
	if !ok || len(data) < 1 {
		t.Fatalf("expected at least 1 file, got %v", lm["data"])
	}
}

func TestCreateAndGetFileLink(t *testing.T) {
	srv, tc := setupStripe(t)

	// Create a file first
	formBody := "purpose=dispute_evidence"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/files", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(r)
	fm := decodeJSONBody(t, resp.Body)
	resp.Body.Close()
	fileID := fm["id"].(string)

	// Create file link
	formBody = "file=" + fileID
	r, _ = http.NewRequest("POST", srv.URL+"/v1/file_links", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	m := decodeJSONBody(t, resp.Body)

	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected file link id")
	}
	if m["object"] != "file_link" {
		t.Errorf("expected object=file_link, got %v", m["object"])
	}
	if m["file"] != fileID {
		t.Errorf("expected file=%s, got %v", fileID, m["file"])
	}

	// Get file link
	getResp := stripeGet(tc, "/v1/file_links/"+id)
	getResp.AssertStatus(200)
	gm := getResp.JSONMap()
	if gm["id"] != id {
		t.Errorf("expected id=%s, got %v", id, gm["id"])
	}
}

func TestCreateAndGetShippingRate(t *testing.T) {
	srv, tc := setupStripe(t)

	formBody := "display_name=Standard+Shipping&fixed_amount[amount]=500&fixed_amount[currency]=usd"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/shipping_rates", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	m := decodeJSONBody(t, resp.Body)

	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected shipping rate id")
	}
	if m["object"] != "shipping_rate" {
		t.Errorf("expected object=shipping_rate, got %v", m["object"])
	}
	if m["display_name"] != "Standard Shipping" {
		t.Errorf("expected display_name=Standard Shipping, got %v", m["display_name"])
	}
	if m["active"] != true {
		t.Errorf("expected active=true, got %v", m["active"])
	}
	if m["type"] != "fixed_amount" {
		t.Errorf("expected type=fixed_amount, got %v", m["type"])
	}

	fa, ok := m["fixed_amount"].(map[string]any)
	if !ok {
		t.Fatal("expected fixed_amount object")
	}
	if fa["amount"] != float64(500) {
		t.Errorf("expected fixed_amount.amount=500, got %v", fa["amount"])
	}
	if fa["currency"] != "usd" {
		t.Errorf("expected fixed_amount.currency=usd, got %v", fa["currency"])
	}

	// Get shipping rate
	getResp := stripeGet(tc, "/v1/shipping_rates/"+id)
	getResp.AssertStatus(200)
	gm := getResp.JSONMap()
	if gm["id"] != id {
		t.Errorf("expected id=%s, got %v", id, gm["id"])
	}
}

func TestUpdateShippingRate(t *testing.T) {
	srv, tc := setupStripe(t)

	// Create
	formBody := "display_name=Express"
	r, _ := http.NewRequest("POST", srv.URL+"/v1/shipping_rates", strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(r)
	cm := decodeJSONBody(t, resp.Body)
	resp.Body.Close()
	id := cm["id"].(string)

	// Update active to false
	formBody = "active=false"
	r, _ = http.NewRequest("POST", srv.URL+"/v1/shipping_rates/"+id, strings.NewReader(formBody))
	r.Header.Set("Authorization", "Bearer sk_test_sim_123")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ = http.DefaultClient.Do(r)
	um := decodeJSONBody(t, resp.Body)
	resp.Body.Close()

	if um["active"] != false {
		t.Errorf("expected active=false after update, got %v", um["active"])
	}

	// Verify via get
	getResp := stripeGet(tc, "/v1/shipping_rates/"+id)
	getResp.AssertStatus(200)
	gm := getResp.JSONMap()
	if gm["active"] != false {
		t.Errorf("expected active=false on get, got %v", gm["active"])
	}
}

// decodeJSONBody reads and decodes a JSON response body.
func decodeJSONBody(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	return m
}

// TestCreateFileRejectsOversizedBody locks in the over-cap behaviour added
// alongside the MaxBytesReader guard: multipart and urlencoded bodies must
// answer the same shape for the same condition, rather than the urlencoded
// path falling through to "Missing required param: purpose."
func TestCreateFileRejectsOversizedBody(t *testing.T) {
	restore := api.SetMaxUploadBodyBytes(1 << 10)
	t.Cleanup(restore)

	srv, _ := setupStripe(t)
	body := strings.Repeat("a", 4<<10) + "&purpose=dispute_evidence"

	for _, tc := range []struct {
		name        string
		contentType string
	}{
		{"urlencoded", "application/x-www-form-urlencoded"},
		{"multipart", "multipart/form-data; boundary=xyz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest("POST", srv.URL+"/v1/files", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer sk_test_sim_123")
			r.Header.Set("Content-Type", tc.contentType)

			resp, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 400 {
				t.Fatalf("expected 400 for an over-cap body, got %d", resp.StatusCode)
			}
			m := decodeJSONBody(t, resp.Body)
			errObj, ok := m["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected a Stripe error envelope, got %v", m)
			}
			if got := errObj["type"]; got != "invalid_request_error" {
				t.Errorf("error.type = %v, want invalid_request_error", got)
			}
			if got := errObj["code"]; got != "parse_error" {
				t.Errorf("error.code = %v, want parse_error", got)
			}
			if msg, _ := errObj["message"].(string); !strings.Contains(msg, "maximum size") {
				t.Errorf("message = %q, want it to mention the maximum size", msg)
			}
		})
	}
}
