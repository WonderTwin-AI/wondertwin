package api

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-resend/internal/store"
)

// maxScheduleAhead is how far ahead Resend lets an email be scheduled.
const maxScheduleAhead = 30 * 24 * time.Hour

// Resend's scheduled_at errors, recorded 2026-11-08 (probes l11 to l13).
const (
	scheduleUnparsable = "The `scheduled_at` must be a valid date either in ISO 8601 format or a valid natural language input like \"in 1 min\""
	schedulePast       = "The `scheduled_at` field must be a future date"
	scheduleTooFar     = "The `scheduled_at` field must be within 30 days"
)

var relativeSchedule = regexp.MustCompile(`^in (\d+) (min|mins|minute|minutes|hour|hours|day|days)$`)

// parseScheduledAt reads scheduled_at as ISO 8601 or as the relative natural
// language form 'in N min|hour|day' against now, and checks it lies in the
// future and within 30 days (quirk lifecycle-003). It returns Resend's error
// message, or "" with the instant.
func parseScheduledAt(raw string, now time.Time) (time.Time, string) {
	at, ok := parseScheduleTime(raw, now)
	switch {
	case !ok:
		return time.Time{}, scheduleUnparsable
	case !at.After(now):
		return time.Time{}, schedulePast
	case at.Sub(now) > maxScheduleAhead:
		return time.Time{}, scheduleTooFar
	}
	return at, ""
}

func parseScheduleTime(raw string, now time.Time) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, true
	}
	m := relativeSchedule.FindStringSubmatch(strings.ToLower(strings.TrimSpace(raw)))
	if m == nil {
		return time.Time{}, false
	}
	const beyondCap = int(maxScheduleAhead/time.Minute) + 1
	n, err := strconv.Atoi(m[1])
	if err != nil || n > beyondCap {
		n = beyondCap
	}
	unit := time.Minute
	switch {
	case strings.HasPrefix(m[2], "hour"):
		unit = time.Hour
	case strings.HasPrefix(m[2], "day"):
		unit = 24 * time.Hour
	}
	return now.Add(time.Duration(n) * unit), true
}

// settleScheduled moves a scheduled email to its recipient outcome once the
// store clock reaches scheduled_at, and reports whether it changed. Resend
// sends a scheduled email at its time; test addresses then settle at once
// (quirk lifecycle-001).
func settleScheduled(e *store.Email, now time.Time) bool {
	if e.LastEvent != store.EmailStatusScheduled || e.ScheduledAt == nil {
		return false
	}
	at, err := time.Parse(pgTimestamp, *e.ScheduledAt)
	if err != nil || now.Before(at) {
		return false
	}
	outcome := testAddressOutcome(e.To)
	e.Status, e.LastEvent = outcome, outcome
	return true
}

// settledEmail returns the stored email, settled against the store clock.
// A scheduled email settles when it is next read, and its email.sent and
// outcome events go out then.
func (h *Handler) settledEmail(e store.Email) store.Email {
	h.settleMu.Lock()
	if cur, ok := h.store.Emails.Get(e.ID); ok {
		e = cur
	}
	settled := settleScheduled(&e, h.store.Clock.Now())
	if settled {
		h.store.Emails.Set(e.ID, e)
	}
	h.settleMu.Unlock()
	if settled {
		h.emitEmailEvents(e)
	}
	return e
}
