package capacity

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestDBCounterDeltas(t *testing.T) {
	previous := DBSnapshot{Counters: DBCounters{
		XactCommit: 100, XactRollback: 20, BlksHit: 900, BlksRead: 100, Deadlocks: 8,
	}}
	current := DBSnapshot{Counters: DBCounters{
		XactCommit: 112, XactRollback: 22, BlksHit: 920, BlksRead: 110, Deadlocks: 9,
	}}

	delta := DBCounterDeltas(previous, current)
	if delta.XactCommit != 12 || delta.XactRollback != 2 || delta.Deadlocks != 1 {
		t.Fatalf("transaction counter deltas = %+v", delta)
	}
	if delta.BlksHit != 20 || delta.BlksRead != 10 || delta.CounterResetDetected {
		t.Fatalf("block counter deltas/reset flag = %+v", delta)
	}
	if delta.SharedCacheHitRatio == nil || *delta.SharedCacheHitRatio != 2.0/3.0 {
		t.Fatalf("shared cache hit ratio = %v, want 2/3", delta.SharedCacheHitRatio)
	}
}

func TestDBCounterDeltasResetMakesRatioUnavailable(t *testing.T) {
	previous := DBSnapshot{Counters: DBCounters{
		XactCommit: 100, XactRollback: 20, BlksHit: 900, BlksRead: 100, Deadlocks: 8,
	}}
	current := DBSnapshot{Counters: DBCounters{
		XactCommit: 112, XactRollback: 22, BlksHit: 30, BlksRead: 110, Deadlocks: 9,
	}}

	delta := DBCounterDeltas(previous, current)
	if !delta.CounterResetDetected {
		t.Fatalf("counter reset was not detected: %+v", delta)
	}
	if delta.BlksHit != 30 || delta.BlksRead != 10 {
		t.Fatalf("counter-reset lower-bound deltas = %+v", delta)
	}
	if delta.SharedCacheHitRatio != nil {
		t.Fatalf("shared cache hit ratio = %v, want unavailable after reset", *delta.SharedCacheHitRatio)
	}
}

func TestDBCounterDeltasWithoutBlockActivity(t *testing.T) {
	delta := DBCounterDeltas(DBSnapshot{}, DBSnapshot{})
	if delta.SharedCacheHitRatio != nil {
		t.Fatalf("shared cache hit ratio = %v, want unavailable", *delta.SharedCacheHitRatio)
	}
}

func TestSnapshotDBIsolatedPostgres(t *testing.T) {
	pool := testdb.New(t)
	snapshot, err := SnapshotDB(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TimestampUTC.IsZero() || snapshot.TimestampUTC.Location().String() != "UTC" {
		t.Fatalf("timestampUTC = %v, want a nonzero UTC timestamp", snapshot.TimestampUTC)
	}
	if snapshot.ConnectionWaits == nil {
		t.Fatal("connection wait-event map is nil")
	}
	for _, queue := range []string{"notification_deliveries", "period_cancellations", "settlement_jobs", "draw_corrections"} {
		if _, ok := snapshot.Queues[queue]; !ok {
			t.Errorf("missing queue %q from migrated test schema", queue)
		}
	}
	if !snapshot.Replication.Available && len(snapshot.Replication.Replicas) != 0 {
		t.Fatalf("replication unavailable with rows: %+v", snapshot.Replication)
	}
}
