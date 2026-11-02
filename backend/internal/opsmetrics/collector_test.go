package opsmetrics

import (
	"context"

	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestCollectOmitsSnapshotBeforeFirstSuccess(t *testing.T) {
	c := New(nil, nil)
	c.mu.Lock()
	c.lastAttempt = time.Now()
	c.lastDuration = 3 * time.Millisecond
	c.lastSuccess = false
	c.mu.Unlock()

	names := metricNames(t, c)
	for _, name := range []string{"lottery_database_up", "lottery_ops_snapshot_success", "lottery_ops_snapshot_timestamp_seconds", "lottery_ops_snapshot_duration_seconds"} {
		if !names[name] {
			t.Errorf("expected metadata metric %q", name)
		}
	}
	for _, name := range []string{"lottery_work_pending", "lottery_work_oldest_pending_age_seconds", "lottery_work_failed", "lottery_replication_lag_bytes"} {
		if names[name] {
			t.Errorf("unexpected uncached metric %q", name)
		}
	}
}

func TestCollectOmitsStaleSnapshot(t *testing.T) {
	c := New(nil, nil)
	c.mu.Lock()
	c.last = snapshot{completedAt: time.Now().Add(-staleAfter - time.Second), workPending: map[string]float64{"notification": 4}}
	c.hasSnapshot = true
	c.lastSuccess = true
	c.lastAttempt = time.Now().Add(-staleAfter - time.Second)
	c.mu.Unlock()

	names := metricNames(t, c)
	if names["lottery_work_pending"] {
		t.Fatal("stale pending gauge was exposed")
	}
}

func TestCanceledRefreshRetainsLastGoodSnapshotButOmitsIt(t *testing.T) {
	c := New(nil, nil)
	completed := time.Now()
	c.mu.Lock()
	c.last = snapshot{completedAt: completed, workPending: map[string]float64{"notification": 7}}
	c.hasSnapshot = true
	c.lastSuccess = true
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Refresh(ctx)

	c.mu.RLock()
	retained := c.hasSnapshot && c.last.completedAt.Equal(completed) && c.last.workPending["notification"] == 7
	success := c.lastSuccess
	c.mu.RUnlock()
	if !retained {
		t.Fatal("failed refresh replaced or discarded last good snapshot")
	}
	if success {
		t.Fatal("canceled refresh reported success")
	}
	if names := metricNames(t, c); names["lottery_work_pending"] {
		t.Fatal("last-good data was exposed as a current snapshot after a failed source")
	}
}

func metricNames(t *testing.T, c *Collector) map[string]bool {
	t.Helper()
	r := prometheus.NewRegistry()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	families, err := r.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(families))
	for _, family := range families {
		out[family.GetName()] = true
	}
	return out
}
