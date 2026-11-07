package api_test

import "testing"

// TestMissingFieldsAnswer422 checks the older create routes answer a missing
// required field as POST /emails does: 422 validation_error naming the field.
func TestMissingFieldsAnswer422(t *testing.T) {
	_, tc := setupResend(t)
	audienceID := resendPost(tc, "/audiences", map[string]any{"name": "General"}).
		AssertStatus(201).JSONMap()["id"].(string)

	cases := []struct {
		path, field string
		body        map[string]any
	}{
		{"/domains", "name", map[string]any{}},
		{"/audiences", "name", map[string]any{}},
		{"/api-keys", "name", map[string]any{}},
		{"/audiences/" + audienceID + "/contacts", "email", map[string]any{}},
		{"/broadcasts", "from", map[string]any{"audience_id": audienceID, "subject": "s"}},
	}
	for _, c := range cases {
		resendPost(tc, c.path, c.body).
			AssertStatus(422).
			AssertBodyContains(`"name":"validation_error"`).
			AssertBodyContains("Missing `" + c.field + "` field.")
	}
}
