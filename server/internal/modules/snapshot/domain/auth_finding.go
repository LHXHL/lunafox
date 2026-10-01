package domain

import "time"

// AuthFindingSnapshot contains only redacted authentication observations.
// Passwords and raw zombie output have no storage column.
type AuthFindingSnapshot struct {
	ID        int
	ScanID    int
	URL       string
	Service   string
	Kind      string
	Account   string
	CreatedAt time.Time
}
