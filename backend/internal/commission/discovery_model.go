package commission

import "time"

// Discovery is the public projection of one order-discovery workflow. It
// deliberately excludes source snapshots and worker bookkeeping.
type Discovery struct {
	ID             string     `json:"id"`
	BrandID        string     `json:"brand_id"`
	State          string     `json:"state"`
	Version        int64      `json:"version"`
	CycleID        *string    `json:"cycle_id"`
	WindowFrom     *time.Time `json:"window_from"`
	WindowTo       *time.Time `json:"window_to"`
	NextCheckAt    time.Time  `json:"next_check_at"`
	LastErrorCode  *string    `json:"last_error_code"`
	LastAuditLogID *string    `json:"last_audit_log_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DiscoveryPage struct {
	BrandID    string      `json:"brand_id"`
	Items      []Discovery `json:"items"`
	TotalCount string      `json:"total_count"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}
