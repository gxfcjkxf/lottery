package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"sort"
	"strings"
	"time"
)

// AllowAttempt uses a persistent fixed window shared across every API instance.
// HTTP must take IP from RemoteAddr, not untrusted forwarded headers.
func (s *Store) AllowAttempt(ctx context.Context, e *mutation.Engine, brand, identifier string, meta Metadata) (bool, error) {
	return s.allowAttempt(ctx, e, brand, identifier, meta, true)
}

// AllowAccountAttempt is only for identities already verified by the server.
// Pre-verification Telegram/captcha requests have no shared account bucket.
func (s *Store) AllowAccountAttempt(ctx context.Context, e *mutation.Engine, brand, identifier string, meta Metadata) (bool, error) {
	return s.allowAttempt(ctx, e, brand, identifier, meta, false)
}

func (s *Store) allowAttempt(ctx context.Context, e *mutation.Engine, brand, identifier string, meta Metadata, includeIP bool) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "DELETE FROM auth_rate_limits WHERE expires_at<now()"); err != nil {
		return false, err
	}
	allowed := true
	type bucket struct {
		key   string
		limit int
	}
	buckets := []bucket{}
	if includeIP {
		buckets = append(buckets, bucket{e.Fingerprint("ip:" + meta.IP), 30})
	}
	if identifier = strings.ToLower(strings.TrimSpace(identifier)); identifier != "" {
		buckets = append(buckets, bucket{e.Fingerprint("account:" + identifier), 10})
	}
	// Deterministic lock order avoids account/IP deadlocks across requests.
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].key < buckets[j].key })
	for _, b := range buckets {
		var count int
		err = tx.QueryRow(ctx, `INSERT INTO auth_rate_limits(bucket_hash,attempt_count,window_start,expires_at) VALUES($1,1,now(),now()+interval '5 minutes')
 ON CONFLICT(bucket_hash) DO UPDATE SET attempt_count=auth_rate_limits.attempt_count+1 RETURNING attempt_count`, b.key).Scan(&count)
		if err != nil {
			return false, err
		}
		if count > b.limit {
			allowed = false
		}
		if count == b.limit+1 {
			_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "auth.rate_limit", ResourceType: "auth", RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"limit": b.limit, "window_seconds": int((5 * time.Minute).Seconds())}})
			if err != nil {
				return false, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return allowed, nil
}
