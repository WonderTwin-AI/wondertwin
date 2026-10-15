package api_test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestMetadataUpdatesMergeAndUnset(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/customers?metadata[a]=1&metadata[b]=2", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)

	// A key sent empty is removed; keys not named are kept.
	resp = stripePost(tc, "/v1/customers/"+id+"?metadata[a]=&metadata[c]=3", nil)
	resp.AssertStatus(200)
	md := resp.JSONMap()["metadata"].(map[string]any)
	if _, ok := md["a"]; ok || md["b"] != "2" || md["c"] != "3" || len(md) != 2 {
		t.Errorf("metadata after merge = %v, want {b:2 c:3}", md)
	}

	// An update without metadata leaves it alone.
	resp = stripePost(tc, "/v1/customers/"+id+"?name=N", nil)
	resp.AssertStatus(200)
	if md := resp.JSONMap()["metadata"].(map[string]any); len(md) != 2 {
		t.Errorf("metadata after an unrelated update = %v", md)
	}

	// metadata= clears every key.
	resp = stripePost(tc, "/v1/customers/"+id+"?metadata=", nil)
	resp.AssertStatus(200)
	if md := resp.JSONMap()["metadata"].(map[string]any); len(md) != 0 {
		t.Errorf("metadata after metadata= is %v, want {}", md)
	}
}

func TestMetadataMergesOnEveryUpdate(t *testing.T) {
	_, tc := setupStripe(t)

	resp := stripePost(tc, "/v1/payment_intents?amount=100&currency=usd&metadata[a]=1&metadata[b]=2", nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)
	resp = stripePost(tc, "/v1/payment_intents/"+id+"?metadata[a]=", nil)
	resp.AssertStatus(200)
	md := resp.JSONMap()["metadata"].(map[string]any)
	if _, ok := md["a"]; ok || md["b"] != "2" {
		t.Errorf("payment_intent metadata = %v, want {b:2}", md)
	}
}

func TestMetadataLimits(t *testing.T) {
	_, tc := setupStripe(t)

	many := url.Values{}
	for i := 0; i < 51; i++ {
		many.Set("metadata[k"+strconv.Itoa(i)+"]", "v")
	}
	for _, c := range []struct {
		name, query, param string
	}{
		{"key too long", "metadata[" + strings.Repeat("k", 41) + "]=v", "metadata[" + strings.Repeat("k", 41) + "]"},
		{"value too long", "metadata[k]=" + strings.Repeat("v", 501), "metadata[k]"},
		{"too many keys", many.Encode(), "metadata"},
	} {
		resp := stripePost(tc, "/v1/customers?"+c.query, nil)
		resp.AssertStatus(400)
		e := resp.JSONMap()["error"].(map[string]any)
		if e["type"] != "invalid_request_error" || e["param"] != c.param {
			t.Errorf("%s: error = %v, want invalid_request_error on %s", c.name, e, c.param)
		}
	}

	// The limits themselves are allowed.
	resp := stripePost(tc, "/v1/customers?metadata["+strings.Repeat("k", 40)+"]="+strings.Repeat("v", 500), nil)
	resp.AssertStatus(200)
}

func TestMetadataKeyCountCountsKeptKeys(t *testing.T) {
	_, tc := setupStripe(t)

	first, second := url.Values{}, url.Values{}
	for i := 0; i < 50; i++ {
		first.Set("metadata[k"+strconv.Itoa(i)+"]", "v")
	}
	second.Set("metadata[extra]", "v")
	resp := stripePost(tc, "/v1/customers?"+first.Encode(), nil)
	resp.AssertStatus(200)
	id := resp.JSONMap()["id"].(string)
	resp = stripePost(tc, "/v1/customers/"+id+"?"+second.Encode(), nil)
	resp.AssertStatus(400)
}
