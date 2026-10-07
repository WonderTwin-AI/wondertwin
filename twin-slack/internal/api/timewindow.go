package api

import (
	"strings"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// timeWindow is the oldest and latest bounds conversations.history and
// replies take. A message on a bound is left out unless inclusive is set
// (conversations.history docs, "Pagination by time").
type timeWindow struct {
	oldest, latest string
	inclusive      bool
}

// newTimeWindow checks the bounds, which are Unix timestamps with an optional
// fraction, and answers the error the docs list for one that is not.
func newTimeWindow(oldest, latest string, inclusive bool) (timeWindow, string) {
	if oldest != "" && !isTimestamp(oldest) {
		return timeWindow{}, "invalid_ts_oldest"
	}
	if latest != "" && !isTimestamp(latest) {
		return timeWindow{}, "invalid_ts_latest"
	}
	return timeWindow{oldest: oldest, latest: latest, inclusive: inclusive}, ""
}

func (w timeWindow) filter(msgs []store.Message) []store.Message {
	if w.oldest == "" && w.latest == "" {
		return msgs
	}
	out := []store.Message{}
	for _, m := range msgs {
		if w.oldest != "" {
			if c := compareTS(m.TS, w.oldest); c < 0 || (c == 0 && !w.inclusive) {
				continue
			}
		}
		if w.latest != "" {
			if c := compareTS(m.TS, w.latest); c > 0 || (c == 0 && !w.inclusive) {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

func isTimestamp(s string) bool {
	sec, frac, _ := strings.Cut(s, ".")
	return sec != "" && allDigits(sec) && allDigits(frac)
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// compareTS compares two timestamps exactly, as decimal numbers, without the
// rounding a float would bring at microsecond precision.
func compareTS(a, b string) int {
	as, af, _ := strings.Cut(a, ".")
	bs, bf, _ := strings.Cut(b, ".")
	as, bs = strings.TrimLeft(as, "0"), strings.TrimLeft(bs, "0")
	if len(as) != len(bs) {
		if len(as) < len(bs) {
			return -1
		}
		return 1
	}
	if c := strings.Compare(as, bs); c != 0 {
		return c
	}
	for len(af) < len(bf) {
		af += "0"
	}
	for len(bf) < len(af) {
		bf += "0"
	}
	return strings.Compare(af, bf)
}
