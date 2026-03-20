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
	To    MessageStatus
	Delay time.Duration
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
			StatusSent:    {To: StatusDelivered, Delay: 500 * time.Millisecond},
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
			StatusSent:    {To: StatusDelivered, Delay: 200 * time.Millisecond},
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
