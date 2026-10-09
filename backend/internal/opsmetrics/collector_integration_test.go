package opsmetrics

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/events"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/prometheus/client_golang/prometheus"
)

func collectedMetric(t *testing.T, c *Collector, name, kind string) (float64, bool) {
	t.Helper()
	r := prometheus.NewRegistry()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	families, err := r.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			matches := kind == ""
			for _, label := range metric.Label {
				if kind != "" && label.GetName() == "kind" {
					matches = label.GetValue() == kind
				}
			}
			if matches && metric.Gauge != nil {
				return metric.Gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

func TestPostgresSnapshotEmptyThenActualNotificationWork(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	c := New(db, nil)
	c.Refresh(ctx)
	if value, ok := collectedMetric(t, c, "lottery_ops_snapshot_success", ""); !ok || value != 1 {
		t.Fatalf("empty migrated database snapshot did not succeed: value=%v present=%v", value, ok)
	}
	if value, ok := collectedMetric(t, c, "lottery_work_pending", "notification"); !ok || value != 0 {
		t.Fatalf("empty database notification count = %v, present=%v", value, ok)
	}
	if value, ok := collectedMetric(t, c, "lottery_reconciliation_observed", ""); !ok || value != 0 {
		t.Fatalf("empty database must report no captured business reconciliation, not a healthy-wallet assertion: value=%v present=%v", value, ok)
	}
	if value, ok := collectedMetric(t, c, "lottery_reconciliation_discrepancies", ""); !ok || value != 0 {
		t.Fatalf("empty database discrepancy count = %v, present=%v", value, ok)
	}
	if value, ok := collectedMetric(t, c, "lottery_replication_stats_visible", ""); !ok || (value != 0 && value != 1) {
		t.Fatalf("replication stats visibility should be explicit: value=%v present=%v", value, ok)
	}
	if value, ok := collectedMetric(t, c, "lottery_replication_observed", ""); !ok || value != 0 {
		t.Fatalf("empty database with no streaming replica should report observed=0: value=%v present=%v", value, ok)
	}
	if _, ok := collectedMetric(t, c, "lottery_replication_lag_bytes", ""); ok {
		t.Fatal("absence of streaming replicas was exposed as zero lag")
	}

	if err := database.Seed(ctx, db, "test"); err != nil {
		t.Fatal(err)
	}
	user, member := ids.New(), ids.New()
	brand := "0199a000-0000-7000-8000-000000000001"
	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, user, "opsmetrics_"+user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, member, brand, user); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = events.Append(ctx, tx, brand, "member.joined", member, member, nil); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	c.Refresh(ctx)
	if value, ok := collectedMetric(t, c, "lottery_work_pending", "notification"); !ok || value != 1 {
		t.Fatalf("actual outbox notification count = %v, present=%v", value, ok)
	}
	var ledgerEntries int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerEntries); err != nil || ledgerEntries != 0 {
		t.Fatalf("notification fixture unexpectedly created financial entries: count=%d err=%v", ledgerEntries, err)
	}
	// Damage only this test's owned schema. A healthy ping with a failed source
	// query must not expose last-good backlog as current, or substitute zero.
	if _, err = db.Exec(ctx, `ALTER TABLE notification_deliveries RENAME TO opsmetrics_owned_unavailable_deliveries`); err != nil {
		t.Fatal(err)
	}
	c.Refresh(ctx)
	if value, ok := collectedMetric(t, c, "lottery_database_up", ""); !ok || value != 1 {
		t.Fatal("source query failure incorrectly changed the primary ping result")
	}
	if value, ok := collectedMetric(t, c, "lottery_ops_snapshot_success", ""); !ok || value != 0 {
		t.Fatal("broken source was exposed as a successful snapshot")
	}
	for _, metric := range []string{"lottery_work_pending", "lottery_work_failed", "lottery_reconciliation_discrepancies", "lottery_replication_lag_bytes"} {
		if _, ok := collectedMetric(t, c, metric, ""); ok {
			t.Fatalf("failed source exposed last-good or synthetic-zero %s", metric)
		}
	}
	if _, err = db.Exec(ctx, `ALTER TABLE opsmetrics_owned_unavailable_deliveries RENAME TO notification_deliveries`); err != nil {
		t.Fatal(err)
	}
	c.Refresh(ctx)
	if value, ok := collectedMetric(t, c, "lottery_work_pending", "notification"); !ok || value != 1 {
		t.Fatal("recovered read source did not restore the actual pending count")
	}
}
