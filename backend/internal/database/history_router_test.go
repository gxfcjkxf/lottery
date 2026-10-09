package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWALPositionParsing(t *testing.T) {
	for raw, want := range map[string]uint64{"0/0": 0, "1/0": 1 << 32, "a/FF": 10<<32 | 255, "FFFFFFFF/FFFFFFFF": ^uint64(0)} {
		got, e := parseLSN(raw)
		if e != nil || got != want {
			t.Fatalf("parse %q = %d err=%v", raw, got, e)
		}
	}
	for _, raw := range []string{"", "0", "/0", "0/", "0/0/0", "100000000/0", "0/100000000", "+1/0", "1/-1", " 1/0", "0/g"} {
		if _, e := parseLSN(raw); e == nil {
			t.Fatalf("accepted invalid WAL position %q", raw)
		}
	}
}
func TestReplicaEligibility(t *testing.T) {
	f := walFence{system: "123", database: "lottery", encoding: "UTF8", schemas: "{pg_catalog,public}", lsn: "1/FF", timeline: 1}
	position := "1/FF"
	type node struct {
		system, db, encoding, schemas string
		replay                        *string
		timeline, recoveryTimeline    uint64
		recovery, paused              bool
	}
	base := node{"123", "lottery", "UTF8", "{pg_catalog,public}", &position, 1, 1, true, false}
	for _, tc := range []struct {
		name   string
		change func(*node)
		want   bool
	}{
		{"exact fence", func(*node) {}, true},
		{"ahead", func(n *node) { s := "2/0"; n.replay = &s }, true},
		{"lagging", func(n *node) { s := "1/FE"; n.replay = &s }, false},
		{"not started", func(n *node) { n.replay = nil }, false},
		{"other cluster", func(n *node) { n.system = "456" }, false},
		{"other database", func(n *node) { n.db = "other" }, false},
		{"wrong encoding", func(n *node) { n.encoding = "SQL_ASCII" }, false},
		{"wrong application schema", func(n *node) { n.schemas = "{pg_catalog,other}" }, false},
		{"promoted", func(n *node) { n.recovery = false }, false},
		{"paused", func(n *node) { n.paused = true }, false},
		{"other checkpoint timeline", func(n *node) { n.timeline = 2 }, false},
		{"other recovery timeline", func(n *node) { n.recoveryTimeline = 2 }, false},
		{"unknown timeline", func(n *node) { n.recoveryTimeline = 0 }, false},
		{"corrupt WAL", func(n *node) { s := "bad"; n.replay = &s }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := base
			tc.change(&n)
			if got := eligible(f, n.system, n.db, n.encoding, n.schemas, n.replay, n.timeline, n.recoveryTimeline, n.recovery, n.paused); got != tc.want {
				t.Fatal("eligibility", got)
			}
		})
	}
}
func TestHistoryRouteWhitelist(t *testing.T) {
	for _, route := range []HistoryRoute{HistoryAudit, HistoryNotification, HistoryCompliance, HistoryPresentation, HistoryDomains, HistoryOperation} {
		if !route.allowed() {
			t.Fatal("missing history route", route)
		}
	}
	for _, route := range []HistoryRoute{"", "audit.view.platform", "wallet", "betting", "notification.template.list", "notification.template.update", "brand_operation.view"} {
		if route.allowed() {
			t.Fatal("unsafe route", route)
		}
	}
}
func TestHistoryRoutingUsesPrimaryOnlyWhenAppropriate(t *testing.T) {
	primary := &stubHistoryTx{}
	for _, router := range []*HistoryRouter{nil, NewHistoryRouter(nil)} {
		out, source, e := router.Read(context.Background(), primary, "wallet", func(tx pgx.Tx) (any, error) {
			if tx != primary {
				t.Fatal("primary tx replaced")
			}
			return "primary", nil
		})
		if e != nil || out != "primary" || source.Replica != 0 {
			t.Fatal(out, source, e)
		}
	}
	for _, router := range []*HistoryRouter{nil, NewHistoryRouter(nil)} {
		called := false
		out, source, err := router.Read(context.Background(), primary, HistoryAudit, func(tx pgx.Tx) (any, error) {
			called = tx == primary
			return "primary", nil
		})
		if err != nil || !called || out != "primary" || source.Replica != 0 {
			t.Fatal("allowlisted route without configured replicas did not use primary", out, source, err)
		}
	}
	called := false
	_, source, err := NewHistoryRouter([]*pgxpool.Pool{nil}).Read(context.Background(), primary, HistoryAudit, func(pgx.Tx) (any, error) {
		called = true
		return "unexpected", nil
	})
	if err == nil || called || source.Reason != "fence_unavailable" {
		t.Fatal("configured routing failure did not return an explicit error", source, err)
	}
	called = false
	out, source, err := NewHistoryRouter([]*pgxpool.Pool{nil}).Read(context.Background(), primary, "wallet", func(tx pgx.Tx) (any, error) {
		called = true
		return "primary", nil
	})
	if err != nil || !called || out != "primary" || source.Reason != "primary_only" {
		t.Fatal("unlisted route did not stay on primary", out, source, err)
	}
}

type stubHistoryTx struct{ pgx.Tx }

func (stubHistoryTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("stub transaction cannot start a fence")
}

func TestHistoryCanceledSelectionDoesNotRunCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	router := NewHistoryRouter([]*pgxpool.Pool{nil})
	_, _, e := router.Read(ctx, &stubHistoryTx{}, HistoryAudit, func(pgx.Tx) (any, error) { t.Fatal("canceled request executed"); return nil, nil })
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
