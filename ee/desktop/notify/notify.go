package notify

import "time"

// Represents notification received from control server; SentAt is set by this consumer after sending.
// For the time being, notifications are per-end user device and not per-user.
type Notification struct {
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	ActionUri  string    `json:"action_uri,omitempty"`
	ID         string    `json:"id"`
	ValidUntil int64     `json:"valid_until"` // timestamp
	SentAt     time.Time `json:"sent_at,omitempty"`
}
