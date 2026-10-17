package api_test

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-xero/internal/api"
)

// sdkMsdate is the pattern xero-python 15.2.0 matches an Accounting date
// against (xero_python/api_client/deserializer.py, MS_DATETIME_RE).
var sdkMsdate = regexp.MustCompile(`^"/Date\((-?\d+)([+-]\d{2,4})?\)/"$`)

func TestMsdateWritesTheFormTheSDKParses(t *testing.T) {
	at := time.Date(2015, 8, 13, 3, 52, 36, 790_000_000, time.UTC)
	b, err := json.Marshal(api.Msdate(at))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"/Date(1439437956790+0000)/"` {
		t.Fatalf("marshal = %s", b)
	}
	if !sdkMsdate.Match(b) {
		t.Fatalf("%s does not match the SDK's msdate pattern", b)
	}
}

func TestMsdateReadsEveryInputForm(t *testing.T) {
	want := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		`"2026-10-16"`:                  want,
		`"2026-10-16T00:00:00"`:         want,
		`"2026-10-16T00:00:00Z"`:        want,
		`"2026-10-16T10:00:00+10:00"`:   want,
		`"2026-10-16T00:00:00.0000000"`: want,
		`"/Date(1792108800000)/"`:       want,
		`"/Date(1792108800000+0000)/"`:  want,
		`"/Date(1792108800000-0500)/"`:  want,
		`"/Date(-86400000+0000)/"`:      time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	for in, exp := range cases {
		var d api.Msdate
		if err := json.Unmarshal([]byte(in), &d); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if !d.Time().Equal(exp) {
			t.Errorf("%s = %s, want %s", in, d.Time(), exp)
		}
	}
}

func TestMsdateRejectsWhatItCannotRead(t *testing.T) {
	for _, in := range []string{`"nope"`, `"/Date(abc)/"`, `"/Date(1792108800000"`, `"2026-13-40"`, `1792108800000`, `true`} {
		var d api.Msdate
		if err := json.Unmarshal([]byte(in), &d); err == nil {
			t.Errorf("%s: expected an error, got %s", in, d.Time())
		}
	}
}

func TestMsdateRoundTripsInAStruct(t *testing.T) {
	type rec struct {
		Date    *api.Msdate `json:"Date,omitempty"`
		DueDate *api.Msdate `json:"DueDate"`
	}
	at := api.Msdate(time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC))
	b, err := json.Marshal(rec{Date: &at})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"Date":"/Date(1792143000000+0000)/","DueDate":null}` {
		t.Fatalf("marshal = %s", b)
	}
	var back rec
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Date == nil || !back.Date.Time().Equal(at.Time()) || back.DueDate != nil {
		t.Fatalf("round trip = %+v", back)
	}
}

func TestMsdateISOTimeIsZonelessWithSevenDigits(t *testing.T) {
	at := api.ISOTime(time.Date(2019, 7, 9, 23, 40, 30, 183_313_000, time.UTC))
	b, err := json.Marshal(at)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"2019-07-09T23:40:30.1833130"` {
		t.Fatalf("marshal = %s", b)
	}
	var back api.ISOTime
	if err := json.Unmarshal(b, &back); err != nil || !back.Time().Equal(at.Time()) {
		t.Fatalf("round trip = %v, %v", back.Time(), err)
	}
}
