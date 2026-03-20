package messaging

import "time"

// Lifecycle defines the state machine for a message channel.
type Lifecycle struct {
	InitialStatus MessageStatus
	Transitions   map[MessageStatus][]MessageStatus
	AutoAdvance   map[MessageStatus]AutoAdvanceConfig
}

// AutoAdvanceConfig controls automatic state progression.
type AutoAdvanceConfig struct {
	// To is the default next status (used when Outcomes is empty).
	To    MessageStatus
	// Delay is the base delay before advancing.
	Delay time.Duration
	// Outcomes defines weighted alternative states.
	// When set, the engine picks a state based on weights instead of using To.
	// If empty, To is used (100% probability).
	Outcomes []WeightedStatus
}

// WeightedStatus is a possible outcome with a relative weight.
type WeightedStatus struct {
	Status MessageStatus `json:"status"`
	Weight float64       `json:"weight"`
}

// EmailLifecycle returns the standard email lifecycle.
func EmailLifecycle() *Lifecycle {
	return &Lifecycle{
		InitialStatus: StatusQueued,
		Transitions: map[MessageStatus][]MessageStatus{
			StatusQueued:    {StatusSending, StatusDropped},
			StatusSending:   {StatusSent, StatusFailed},
			StatusSent:      {StatusDelivered, StatusBounced, StatusFailed},
			StatusDelivered: {StatusOpened, StatusComplained},
			StatusOpened:    {StatusClicked},
		},
		AutoAdvance: map[MessageStatus]AutoAdvanceConfig{
			StatusQueued:  {To: StatusSending, Delay: 0},
			StatusSending: {To: StatusSent, Delay: 100 * time.Millisecond},
			StatusSent: {
				To:    StatusDelivered,
				Delay: 500 * time.Millisecond,
				Outcomes: []WeightedStatus{
					{Status: StatusDelivered, Weight: 95},
					{Status: StatusBounced, Weight: 3},
					{Status: StatusFailed, Weight: 2},
				},
			},
		},
	}
}

// SMSLifecycle returns the standard SMS lifecycle.
func SMSLifecycle() *Lifecycle {
	return &Lifecycle{
		InitialStatus: StatusQueued,
		Transitions: map[MessageStatus][]MessageStatus{
			StatusQueued:  {StatusSending, StatusDropped},
			StatusSending: {StatusSent, StatusFailed},
			StatusSent:    {StatusDelivered, StatusUndelivered, StatusFailed},
		},
		AutoAdvance: map[MessageStatus]AutoAdvanceConfig{
			StatusQueued:  {To: StatusSending, Delay: 0},
			StatusSending: {To: StatusSent, Delay: 50 * time.Millisecond},
			StatusSent: {
				To:    StatusDelivered,
				Delay: 200 * time.Millisecond,
				Outcomes: []WeightedStatus{
					{Status: StatusDelivered, Weight: 95},
					{Status: StatusUndelivered, Weight: 3},
					{Status: StatusFailed, Weight: 2},
				},
			},
		},
	}
}

// VoiceLifecycle returns a voice call lifecycle.
func VoiceLifecycle() *Lifecycle {
	return &Lifecycle{
		InitialStatus: StatusQueued,
		Transitions: map[MessageStatus][]MessageStatus{
			StatusQueued:  {StatusSending},
			StatusSending: {StatusSent, StatusFailed},         // ringing
			StatusSent:    {StatusDelivered, StatusFailed},     // in-progress → completed
		},
		AutoAdvance: map[MessageStatus]AutoAdvanceConfig{
			StatusQueued:  {To: StatusSending, Delay: 0},
			StatusSending: {To: StatusSent, Delay: 500 * time.Millisecond},
		},
	}
}
