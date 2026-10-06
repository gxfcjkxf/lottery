package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestConcurrentExceptionAndCancellationHaveOneVersionWinner(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	o, err := placeBettingOrder(t, f, f.input, "exception-cancel-race")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	actor := exceptionActor(f)
	for _, mark := range []bool{true, false} {
		go func(mark bool) {
			<-start
			tx, e := f.db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			if mark {
				_, e = f.service.MarkAbnormal(ctx, tx, f.brand, actor, o.ID, 1, "race exception evidence", policyMeta(actor.ID))
			} else {
				_, e = f.service.CancelAdmin(ctx, tx, f.brand, actor, o.ID, 1, "race original source refund", policyMeta(actor.ID))
			}
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(mark)
	}
	close(start)
	success, conflicts := 0, 0
	for range 2 {
		e := <-results
		if e == nil {
			success++
		} else if errors.Is(e, ErrVersion) {
			conflicts++
		} else {
			t.Fatalf("unexpected racing operation failure %v", e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success/conflicts %d/%d", success, conflicts)
	}
	loaded, err := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || loaded.Version != 2 {
		t.Fatalf("non-atomic race %+v %v", loaded, err)
	}
	evidence, err := f.service.Exception(ctx, f.brand, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status == "abnormal" {
		if evidence == nil || walletBySource(t, f)[0][0] != 9 {
			t.Fatal("mark winner lost evidence or changed money")
		}
		bettingTx(t, f.db, func(tx pgx.Tx) error {
			_, e := f.service.CancelAdmin(ctx, tx, f.brand, actor, o.ID, 2, "refund after winning exception race", policyMeta(actor.ID))
			return e
		})
	} else if loaded.Status != "bet_cancelled" || evidence != nil {
		t.Fatalf("cancel winner left orphan evidence %+v %+v", loaded, evidence)
	}
	if walletBySource(t, f)[0][0] != 10 {
		t.Fatal("race did not refund exactly once")
	}
}

func exceptionActor(f bettingFixture) access.Account {
	return access.Account{ID: f.version.CreatedBy, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "bet", Action: "mark_abnormal", Scope: access.ScopeBrand}, {Resource: "bet", Action: "cancel", Scope: access.ScopeBrand}}}}}
}

func TestManualExceptionEvidenceIsImmutableAndOperatorCanCancel(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	in := f.input
	in.Multiplier = 3
	o, err := placeBettingOrder(t, f, in, "manual-exception-bet")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before := walletBySource(t, f)
	actor := exceptionActor(f)
	var abnormal Order
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		abnormal, e = f.service.MarkAbnormal(ctx, tx, f.brand, actor, o.ID, o.Version, "verified invalid betting data", policyMeta(actor.ID))
		return e
	})
	if abnormal.Status != "abnormal" || abnormal.Version != 2 || abnormal.TotalPoints != o.TotalPoints || walletBySource(t, f) != before {
		t.Fatalf("abnormal changed immutable money: %+v", abnormal)
	}
	evidence, err := f.service.Exception(ctx, f.brand, o.ID)
	if err != nil || evidence == nil || evidence.OrderVersion != abnormal.Version || evidence.MarkedBy != actor.ID || evidence.Reason != "verified invalid betting data" {
		t.Fatalf("missing exception evidence %+v err=%v", evidence, err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.MarkAbnormal(ctx, tx, f.brand, actor, o.ID, 2, "cannot reclassify exception", policyMeta(actor.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrState) {
		t.Fatalf("second classification=%v", err)
	}
	tx, err = f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Cancel(ctx, tx, f.brand, f.user, o.ID, 2, "user cannot cancel exception", points.Metadata{RequestID: ids.New()})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrState) {
		t.Fatalf("ordinary user acted on exception: %v", err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		out, e := f.service.CancelAdmin(ctx, tx, f.brand, actor, o.ID, 2, "operator cancels invalid stake", policyMeta(actor.ID))
		if e == nil && (out.Status != "bet_cancelled" || out.Version != 3) {
			t.Fatalf("cancel=%+v", out)
		}
		return e
	})
	if walletBySource(t, f)[0][0] != 10 {
		t.Fatal("operator refund did not restore source")
	}
	if _, err = f.db.Exec(ctx, `UPDATE bet_order_exceptions SET reason='rewrite' WHERE order_id=$1`, o.ID); err == nil {
		t.Fatal("exception evidence was editable")
	}
	if _, err = f.db.Exec(ctx, `DELETE FROM bet_order_exceptions WHERE order_id=$1`, o.ID); err == nil {
		t.Fatal("exception evidence was deletable")
	}
	var audits, events int
	if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action='bet.mark_abnormal'),(SELECT count(*) FROM outbox_events WHERE event_type='bet.order.abnormal')`).Scan(&audits, &events); err != nil || audits != 1 || events != 1 {
		t.Fatalf("audit/events %d/%d err=%v", audits, events, err)
	}
}

func TestMarkExceptionRejectsMissingAuthorityForeignBrandsAndStaleVersions(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	o, err := placeBettingOrder(t, f, f.input, "exception-authorization-bet")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	actor := exceptionActor(f)
	for _, tc := range []struct {
		name, brand, reason string
		actor               access.Account
		version             int64
		want                error
	}{
		{"no grant", f.brand, "reason", access.Account{ID: actor.ID, Type: access.AccountAdmin, BrandIDs: actor.BrandIDs}, 1, ErrDenied},
		{"platform readonly", f.brand, "reason", func() access.Account { a := actor; a.SuperAdmin = true; return a }(), 1, ErrDenied},
		{"stale", f.brand, "reason", actor, 2, ErrVersion},
		{"missing reason", f.brand, "", actor, 1, ErrInvalid},
		{"foreign brand", storeTestOtherBrand, "reason", func() access.Account {
			a := actor
			a.BrandIDs = []string{storeTestOtherBrand}
			a.Roles = append([]access.Role{}, actor.Roles...)
			a.Roles[0].BrandID = storeTestOtherBrand
			return a
		}(), 1, ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, e := f.db.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			_, e = f.service.MarkAbnormal(ctx, tx, tc.brand, tc.actor, o.ID, tc.version, tc.reason, policyMeta(actor.ID))
			if !errors.Is(e, tc.want) {
				t.Fatalf("got %v want %v", e, tc.want)
			}
		})
	}
	evidence, err := f.service.Exception(ctx, f.brand, o.ID)
	if err != nil || evidence != nil {
		t.Fatalf("rejected mark persisted evidence %+v %v", evidence, err)
	}
	if _, err = f.service.Exception(ctx, storeTestOtherBrand, o.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign exception read %v", err)
	}
}

func TestExceptionEventFailureRollsBackStateAndEvidence(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	o, err := placeBettingOrder(t, f, f.input, "exception-rollback-bet")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = f.db.Exec(ctx, `CREATE FUNCTION reject_exception_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='bet.order.abnormal' THEN RAISE EXCEPTION 'test evidence rollback'; END IF; RETURN NEW; END $$;CREATE TRIGGER reject_exception_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_exception_event()`); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), o.ID, 1, "outbox should rollback evidence", policyMeta(f.version.CreatedBy))
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("expected outbox failure")
	}
	loaded, err := f.service.Order(ctx, f.brand, f.member, o.ID)
	if err != nil || loaded.Status != "placed" || loaded.Version != 1 {
		t.Fatalf("partial classification %+v %v", loaded, err)
	}
	evidence, err := f.service.Exception(ctx, f.brand, o.ID)
	if err != nil || evidence != nil {
		t.Fatalf("partial evidence %+v %v", evidence, err)
	}
}

func TestDatabaseRejectsOrphanEvidenceAndDirectUnwitnessedClassification(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 10)
	o, err := placeBettingOrder(t, f, f.input, "exception-database-guard")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = f.db.Exec(ctx, `UPDATE bet_orders SET status='abnormal',version=version+1 WHERE id=$1`, o.ID); err == nil {
		t.Fatal("state changed without exception evidence")
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO bet_order_exceptions(id,brand_id,order_id,order_version,marked_by,reason) VALUES($1,$2,$3,2,$4,'orphan insert')`, ids.New(), f.brand, o.ID, f.version.CreatedBy); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("orphan evidence committed without classification")
	}
	e, err := f.service.Exception(ctx, f.brand, o.ID)
	if err != nil || e != nil {
		t.Fatalf("orphan evidence persisted %+v %v", e, err)
	}
}
