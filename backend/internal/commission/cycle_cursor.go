package commission

import "time"

// CycleCursor is a keyset position in the immutable bet-placement ordering.
// It is internal worker state, not a public or financial authorization token.
type CycleCursor struct {
	PlacedAt time.Time
	OrderID  string
}
