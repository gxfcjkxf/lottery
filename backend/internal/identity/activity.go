package identity

import "context"

// The existing authenticated /me heartbeat records presence. Keep activity out
// of AuthenticateTx: financial authorization and read-only checks stay unchanged.
func (s *Store) RecordActivity(ctx context.Context, sessionID string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO session_activity(session_id) VALUES($1)
 ON CONFLICT(session_id) DO UPDATE SET last_seen_at=clock_timestamp()
 WHERE session_activity.last_seen_at<clock_timestamp()-interval '30 seconds'`, sessionID)
	return err
}
