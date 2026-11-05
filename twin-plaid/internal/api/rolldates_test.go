package api_test

import (
	"testing"
)

// TestRollDatesForwardFollowsTheEmulatorClock is the demo's flow: one fixed
// configuration dated from an epoch, the clock advanced 90 days, then the link.
func TestRollDatesForwardFollowsTheEmulatorClock(t *testing.T) {
	tc, ac, mem := setupPlaid(t)
	ac.AdvanceTime("2160h").AssertStatus(200)
	today := mem.Clock.Now().UTC().Format("2006-01-02")

	token := customItem(t, tc, `{"roll_dates_forward": true, "override_accounts": [{"type": "depository", "subtype": "checking", "transactions": [
		{"date_transacted": "2020-01-01", "date_posted": "2020-01-01", "amount": -500, "description": "STRIPE PAYOUT"},
		{"date_transacted": "2020-03-30", "date_posted": "2020-03-30", "amount": -500, "description": "STRIPE PAYOUT"},
		{"date_transacted": "2020-03-30", "date_posted": "2020-04-01", "amount": 75, "description": "Card hold"}]}]}`)
	rows := func() map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, a := range tc.Post("/transactions/sync", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()["added"].([]any) {
			r := a.(map[string]any)
			out[r["name"].(string)+" "+r["authorized_date"].(string)] = r
		}
		return out
	}
	got := rows()
	first := mem.Clock.Now().UTC().AddDate(0, 0, -89).Format("2006-01-02")
	posted := mem.Clock.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
	if len(got) != 3 || got["STRIPE PAYOUT "+today] == nil || got["STRIPE PAYOUT "+first] == nil {
		t.Fatalf("rows = %v; want the latest authorized_date on %s and the first 89 days before", got, today)
	}
	if hold := got["Card hold "+today]; hold == nil || hold["date"] != posted || hold["pending"] != true {
		t.Fatalf("hold = %v; want date_posted two days after today and pending", hold)
	}
	if got["STRIPE PAYOUT "+today]["pending"] != false {
		t.Fatal("a row posted today is posted")
	}

	// The shift happened once: advancing the clock later moves nothing.
	ac.AdvanceTime("240h").AssertStatus(200)
	if later := rows(); later["STRIPE PAYOUT "+today] == nil {
		t.Fatalf("dates moved after creation: %v", later)
	}
}

func TestRollDatesForwardOffKeepsTheDatesAsGiven(t *testing.T) {
	tc, _, _ := setupPlaid(t)
	token := customItem(t, tc, `{"override_accounts": [{"type": "depository", "subtype": "checking", "transactions": [
		{"date_transacted": "2020-03-30", "date_posted": "2020-03-30", "amount": 1, "description": "old"}]}]}`)
	r := tc.Post("/transactions/sync", creds(map[string]any{"access_token": token})).AssertStatus(200).JSONMap()["added"].([]any)[0].(map[string]any)
	if r["date"] != "2020-03-30" || r["authorized_date"] != "2020-03-30" {
		t.Fatalf("row = %v", r)
	}
}
