package agency

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

func TestAgentModeCapsAreIndependent(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	cfg := PolicyConfig{Enabled: true, MaxDepth: 5, LossRatioCap: "0.2", TurnoverRatioCap: "0.03", Mode: "loss", Cycle: "weekly"}
	var p Policy
	save := func(c PolicyConfig) error {
		return transaction(t, s, func(tx pgx.Tx) error {
			out, e := s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: max(int64(1), p.Version), Config: c}, metadata(a))
			if e == nil {
				p = out
			}
			return e
		})
	}
	if e := save(cfg); e != nil {
		t.Fatal(e)
	}
	add := func(mode, ratio string) (Node, error) {
		var n Node
		e := transaction(t, s, func(tx pgx.Tx) error {
			var e error
			n, e = s.Create(ctx, tx, brandA, a, CreateInput{PolicyVersion: p.Version, Username: "caps_" + strings.ReplaceAll(ids.New(), "-", "")[8:], Password: "agent123", Config: NodeConfig{Ratio: ratio, Mode: &mode, Status: "active", CanCreateChildren: true}}, metadata(a))
			return e
		})
		return n, e
	}
	loss, e := add("loss", "0.15")
	if e != nil {
		t.Fatal("loss incorrectly used turnover cap", e)
	}
	turnover, e := add("turnover", "0.03")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = add("turnover", "0.04"); !errors.Is(e, ErrLimit) {
		t.Fatal("turnover cap ignored", e)
	}
	if _, e = add("loss", "0.21"); !errors.Is(e, ErrLimit) {
		t.Fatal("loss cap ignored", e)
	}
	// Lowering one cap checks nodes of that mode only.
	cfg.TurnoverRatioCap = "0.04"
	if e = save(cfg); e != nil {
		t.Fatal("loss nodes incorrectly checked against turnover cap", e)
	}
	cfg.LossRatioCap = "0.14"
	if e = save(cfg); !errors.Is(e, ErrLimit) {
		t.Fatal("existing loss node not protected", e)
	}
	cfg.LossRatioCap = "0.2"
	cfg.TurnoverRatioCap = "0.02"
	if e = save(cfg); !errors.Is(e, ErrLimit) {
		t.Fatal("existing turnover node not protected", e)
	}
	// Updates retain their line's cap, including when the brand default differs.
	for _, tc := range []struct {
		n   Node
		r   string
		bad bool
	}{{loss, "0.19", false}, {turnover, "0.05", true}} {
		e = transaction(t, s, func(tx pgx.Tx) error {
			c := tc.n.Config
			c.Ratio = tc.r
			_, e := s.Update(ctx, tx, brandA, a, tc.n.ID, UpdateInput{Version: tc.n.Version, PolicyVersion: p.Version, Config: c}, metadata(a))
			return e
		})
		if tc.bad && !errors.Is(e, ErrLimit) || !tc.bad && e != nil {
			t.Fatal("update cap", e)
		}
	}
	// Direct SQL cannot bypass either mode's cap (not merely Go validation).
	for _, tc := range []struct {
		n Node
		r string
	}{{loss, "0.21"}, {turnover, "0.05"}} {
		e = transaction(t, s, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=jsonb_set(config,'{ratio}',to_jsonb($2::text)) WHERE id=$1`, tc.n.ID, tc.r)
			return e
		})
		if e == nil {
			t.Fatal("SQL mode cap bypass")
		}
	}
}
