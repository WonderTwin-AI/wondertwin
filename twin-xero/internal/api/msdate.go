package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Xero writes dates two ways, by API family (quirks time-001, time-002).
// Accounting dates (the spec's x-is-msdate and x-is-msdate-time fields) are
// /Date(ms+0000)/ strings, and xero-python raises ValueError on an ISO string
// there. Connections and webhook dates are ISO date-times with no zone.

// msdateRE is the form xero-python's deserializer accepts: milliseconds since
// the epoch with an optional offset, which it only uses to label the zone.
var msdateRE = regexp.MustCompile(`^/Date\((-?\d+)([+-]\d{2,4})?\)/$`)

// Msdate is an Accounting date. It writes /Date(ms+0000)/ and reads that form,
// YYYY-MM-DD and ISO date-times, a zoneless one being UTC.
type Msdate time.Time

// Time returns the date as a time.Time.
func (d Msdate) Time() time.Time { return time.Time(d) }

// MarshalJSON writes the /Date(ms+0000)/ form, and null for the zero time.
func (d Msdate) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(fmt.Sprintf("/Date(%d+0000)/", t.UnixMilli()))
}

// UnmarshalJSON reads a date in any form Xero accepts on input.
func (d *Msdate) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return &badValueError{Raw: append([]byte(nil), b...)}
	}
	if m := msdateRE.FindStringSubmatch(s); m != nil {
		ms, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return &badValueError{Raw: append([]byte(nil), b...)}
		}
		*d = Msdate(time.UnixMilli(ms).UTC())
		return nil
	}
	t, err := parseISO(s)
	if err != nil {
		return &badValueError{Raw: append([]byte(nil), b...)}
	}
	*d = Msdate(t)
	return nil
}

// ISOTime is a Connections or webhook date: an ISO date-time with seven
// fractional digits and no zone, in UTC.
type ISOTime time.Time

// Time returns the date as a time.Time.
func (d ISOTime) Time() time.Time { return time.Time(d) }

// MarshalJSON writes 2006-01-02T15:04:05.0000000, and null for the zero time.
func (d ISOTime) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format("2006-01-02T15:04:05.0000000"))
}

// UnmarshalJSON reads an ISO date or date-time.
func (d *ISOTime) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return &badValueError{Raw: append([]byte(nil), b...)}
	}
	t, err := parseISO(s)
	if err != nil {
		return &badValueError{Raw: append([]byte(nil), b...)}
	}
	*d = ISOTime(t)
	return nil
}

var isoLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

// parseISO reads YYYY-MM-DD or an ISO date-time. A value with no zone is UTC,
// as Xero treats it.
func parseISO(s string) (time.Time, error) {
	for _, layout := range isoLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q", s)
}
