package rulebook

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

const brand = "0199a000-0000-7000-8000-000000000001"

func actor(id string) access.Account {
	a := access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}}
	r := access.Role{BrandID: brand}
	for _, p := range []struct{ resource, action string }{{"game", "write"}, {"rule", "write"}, {"rule", "validate"}, {"rule", "submit"}, {"rule", "review"}} {
		r.Permissions = append(r.Permissions, access.Permission{Resource: p.resource, Action: p.action, Scope: access.ScopeBrand})
	}
	a.Roles = []access.Role{r}
	return a
}
func fixture(t *testing.T) (Store, access.Account, access.Account, Game, Play, rules.Definition) {
	t.Helper()
	db := testdb.New(t)
	s := Store{DB: db}
	ctx := context.Background()
	a, b := actor(ids.New()), actor(ids.New())
	for i, x := range []access.Account{a, b} {
		_, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, x.ID, []string{"rule_creator", "rule_reviewer"}[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	d := rules.Definition{SchemaVersion: 1, Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1, PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: ptr(3)}, Odds: "10", Exclusive: true}}, Rounding: "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	var g Game
	var p Play
	transact(t, db, func(tx pgx.Tx) error {
		var err error
		g, err = s.CreateGame(ctx, tx, brand, a, "daily_three", "Daily Three", d.Model, "Asia/Manila", "create game", points.Metadata{})
		return err
	})
	transact(t, db, func(tx pgx.Tx) error {
		var err error
		p, err = s.CreatePlay(ctx, tx, brand, a, g.ID, "exact", "Exact", "create play", points.Metadata{})
		return err
	})
	return s, a, b, g, p, d
}
func ptr[T any](v T) *T { return &v }

type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func transact(t *testing.T, db beginner, fn func(pgx.Tx) error) {
	t.Helper()
	tx, e := db.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if e = fn(tx); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func testCase() rules.ValidationCase {
	return rules.ValidationCase{Name: "121", Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: ptr(points.Amount(1)), ExpectedPrizePoints: ptr(points.Amount(10)), ExpectedWon: ptr(true)}
}
func draftReady(t *testing.T, s Store, a access.Account, p Play, d rules.Definition, mode string) Version {
	t.Helper()
	ctx := context.Background()
	var v Version
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.CreateVersion(ctx, tx, brand, a, p.ID, d, mode, "draft", points.Metadata{})
		return e
	})
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Validate(ctx, tx, brand, a, v.ID, v.Version, []rules.ValidationCase{testCase()}, "validate", points.Metadata{})
		return e
	})
	if v.Validation == nil || !v.Validation.Passed {
		t.Fatal(v.Validation)
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Submit(ctx, tx, brand, a, v.ID, v.Version, "submit", points.Metadata{})
		return e
	})
	return v
}
func TestImmediateApprovalImmutableHistoryAndNoSelfReview(t *testing.T) {
	s, a, b, _, p, d := fixture(t)
	ctx := context.Background()
	v := draftReady(t, s, a, p, d, "immediate")
	tx, _ := s.DB.Begin(ctx)
	_, e := s.Review(ctx, tx, brand, a, v.ID, v.Version, true, true, "approve self", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Review(ctx, tx, brand, b, v.ID, v.Version, true, true, "approve", points.Metadata{})
		return e
	})
	if v.Status != "active" || v.EffectiveAt == nil {
		t.Fatal(v)
	}
	tx, _ = s.DB.Begin(ctx)
	_, e = s.UpdateVersion(ctx, tx, brand, a, v.ID, v.Version, d, "immediate", "edit active", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	tx, _ = s.DB.Begin(ctx)
	_, e = tx.Exec(ctx, `UPDATE rule_versions SET definition='{}'::jsonb,version=version+1 WHERE id=$1`, v.ID)
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("active definition rewrite accepted")
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Clone(ctx, tx, brand, a, v.ID, "immediate", "rollback draft", points.Metadata{})
		return e
	})
	if v.Status != "draft" || v.VersionNo != 2 || v.SourceVersionID == "" {
		t.Fatal(v)
	}
}
func TestNextPeriodActivationAndApprovalConflict(t *testing.T) {
	s, a, b, g, p, d := fixture(t)
	ctx := context.Background()
	v := draftReady(t, s, a, p, d, "next_period")
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Review(ctx, tx, brand, b, v.ID, v.Version, true, true, "next period", points.Metadata{})
		return e
	})
	if v.Status != "approved" || v.EffectiveAt != nil || v.EffectiveSequence == nil || *v.EffectiveSequence != 1 {
		t.Fatal(v)
	}
	plays, e := s.Plays(ctx, brand, g.ID, 100, 0)
	if e != nil || plays[0].ActiveVersionID != "" {
		t.Fatal(plays, e)
	}
	other := draftReady(t, s, a, p, d, "immediate")
	tx, _ := s.DB.Begin(ctx)
	_, e = s.Review(ctx, tx, brand, b, other.ID, other.Version, true, true, "must not replace queue", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	now := time.Now()
	var period Period
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		period, e = s.OpenPeriod(ctx, tx, brand, g.ID, "20261006001", now.Add(-time.Minute), now.Add(time.Minute), now.Add(2*time.Minute))
		return e
	})
	current, e := s.GetVersion(ctx, brand, v.ID)
	if e != nil || current.Status != "active" || current.EffectivePeriodID != period.ID {
		t.Fatal(current, e)
	}
	var n int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND rule_version_id=$2`, period.ID, v.ID).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	tx, _ = s.DB.Begin(ctx)
	_, e = s.OpenPeriod(ctx, tx, brand, g.ID, "future", now.Add(time.Hour), now.Add(2*time.Hour), now.Add(3*time.Hour))
	tx.Rollback(ctx)
	if !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestDraftValidationInvalidateAndBrandModelBoundaries(t *testing.T) {
	s, a, b, _, p, d := fixture(t)
	ctx := context.Background()
	v := draftReady(t, s, a, p, d, "immediate")
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Review(ctx, tx, brand, b, v.ID, v.Version, false, false, "wrong odds", points.Metadata{})
		return e
	})
	if v.Status != "rejected" {
		t.Fatal(v)
	}
	tx, _ := s.DB.Begin(ctx)
	_, e := s.UpdateVersion(ctx, tx, brand, a, v.ID, v.Version, d, "immediate", "no edit rejected", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.CreateVersion(ctx, tx, brand, a, p.ID, d, "immediate", "new draft", points.Metadata{})
		return e
	})
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.Validate(ctx, tx, brand, a, v.ID, v.Version, []rules.ValidationCase{testCase()}, "validate", points.Metadata{})
		return e
	})
	d.PrizeTiers[0].Odds = "11"
	transact(t, s.DB, func(tx pgx.Tx) error {
		var e error
		v, e = s.UpdateVersion(ctx, tx, brand, a, v.ID, v.Version, d, "immediate", "change odds", points.Metadata{})
		return e
	})
	if v.Validation != nil {
		t.Fatal("validation survived edit")
	}
	tx, _ = s.DB.Begin(ctx)
	_, e = s.Submit(ctx, tx, brand, a, v.ID, v.Version, "without validation", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrValidation) {
		t.Fatal(e)
	}
	tx, _ = s.DB.Begin(ctx)
	_, e = s.UpdateVersion(ctx, tx, brand, a, v.ID, v.Version-1, d, "immediate", "stale", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	a.SuperAdmin = true
	tx, _ = s.DB.Begin(ctx)
	_, e = s.CreateVersion(ctx, tx, brand, a, p.ID, d, "immediate", "super write", points.Metadata{})
	tx.Rollback(ctx)
	if !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	foreign := "0199a000-0000-7000-8000-000000000002"
	if _, e = s.GetVersion(ctx, foreign, v.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}
