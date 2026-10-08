package reportarchive

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
)

const testBrand = "0199a000-0000-7000-8000-000000000001"
const otherBrand = "0199a000-0000-7000-8000-000000000002"

func archiveFixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	s := Service{DB: testdb.New(t)}
	id := ids.New()
	if _, err := s.DB.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'synthetic-archive-only')`, id, "archive_"+strings.ReplaceAll(id, "-", "")); err != nil {
		t.Fatal(err)
	}
	a := access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{{Resource: "report_archive", Action: "view", Scope: access.ScopeBrand}, {Resource: "report_archive", Action: "create", Scope: access.ScopeBrand}}}}}
	return s, a
}
func archiveMeta(a access.Account) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
}
func pastInput() Input {
	return Input{Kind: Daily, PeriodKey: time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02"), ExpectedRevision: 0, Reason: "capture elapsed report observation"}
}
func archiveCreate(t *testing.T, s Service, a access.Account, in Input) Record {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	r, err := s.CreateTx(ctx, tx, testBrand, a, in, archiveMeta(a))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return r
}
func archiveRead(t *testing.T, s Service, id string) Record {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	r, err := s.ReadTx(ctx, tx, testBrand, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func archiveMoney(t *testing.T, s Service) string {
	t.Helper()
	var v string
	if err := s.DB.QueryRow(context.Background(), `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_accounts p),'buckets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY account_id,source,state) FROM point_buckets p),'ledger',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_ledger_entries p),'recharges',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM recharge_orders p),'bets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM bet_orders p),'outbox',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM outbox_events p))::text)`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
func archiveFund(t *testing.T, s Service) {
	t.Helper()
	ctx := context.Background()
	u, m, a := ids.New(), ids.New(), ids.New()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, item := range []struct {
		q string
		v []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{u, "archive_member_" + u}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{m, testBrand, u}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{a, testBrand, m}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, []any{testBrand, a}},
	} {
		if _, err = tx.Exec(ctx, item.q, item.v...); err != nil {
			t.Fatal(err)
		}
	}
	var delta points.Balance
	delta[0][0] = 13
	if _, err = (points.Store{DB: s.DB}).Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: m, EntryType: "adjustment", ReferenceType: "archive_test", OperationKey: "archive-fund:" + ids.New(), Reason: "authentic present-state wallet credit", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 13}}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveVersionsFreezePastWindowAndNeverMoveMoney(t *testing.T) {
	s, a := archiveFixture(t)
	in := pastInput()
	before := archiveMoney(t, s)
	first := archiveCreate(t, s, a, in)
	if first.Revision != 1 || first.PreviousID != nil || first.Snapshot.FormatVersion != 1 || len(first.PayloadSHA256) != 64 || !first.SnapshotAt.Equal(first.CreatedAt) || !first.SnapshotAt.Equal(first.Snapshot.SnapshotAt) || !first.SnapshotAt.Equal(first.Snapshot.WalletSnapshot.AtSnapshot) || !first.Window.From.Equal(first.Snapshot.From) || !first.Window.To.Equal(first.Snapshot.To) {
		t.Fatalf("invalid first archive: %+v", first)
	}
	if archiveMoney(t, s) != before {
		t.Fatal("archive creation moved financial data")
	}
	archiveFund(t, s)
	if got := archiveRead(t, s, first.ID); !reflect.DeepEqual(got, first) {
		t.Fatal("later genuine credit rewrote old archive")
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE brands SET timezone='America/New_York' WHERE id=$1`, testBrand); err != nil {
		t.Fatal(err)
	}
	in.ExpectedRevision = 1
	in.Reason = "explicit new observation after current-wallet change"
	second := archiveCreate(t, s, a, in)
	if second.Revision != 2 || second.PreviousID == nil || *second.PreviousID != first.ID || !reflect.DeepEqual(first.Window, second.Window) || second.Snapshot.Timezone != first.Window.Timezone {
		t.Fatalf("new timezone retagged archived period: first=%+v second=%+v", first, second)
	}
	if first.Snapshot.WalletSnapshot.Balances.TotalPoints != "0" || second.Snapshot.WalletSnapshot.Balances.TotalPoints != "13" || second.Snapshot.Ledger.NetPoints != "0" {
		t.Fatal("current wallet observation confused with past-period posting", first.Snapshot, second.Snapshot)
	}
	before = archiveMoney(t, s)
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	page, err := s.ListTx(ctx, tx, testBrand, 20, 0)
	if err != nil || page.TotalCount != "2" || len(page.Items) != 2 {
		t.Fatal(page, err)
	}
	if _, err = s.ReadTx(ctx, tx, otherBrand, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-brand archive leak", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE report_archives SET reason='changed'`, `DELETE FROM report_archives`, `TRUNCATE report_archives`} {
		_, err = s.DB.Exec(ctx, q)
		var rejected *pgconn.PgError
		if !errors.As(err, &rejected) || rejected.Message != "report archives are immutable" {
			t.Fatal("archive mutation not rejected by immutable guard", q, err)
		}
	}
	if archiveMoney(t, s) != before {
		t.Fatal("archive reads mutated funds")
	}
	monthly := Input{Kind: Monthly, PeriodKey: time.Now().UTC().AddDate(0, -2, 0).Format("2006-01"), Reason: "closed calendar month observation"}
	month := archiveCreate(t, s, a, monthly)
	if month.Window.Kind != Monthly || month.Window.Timezone != "America/New_York" || month.Revision != 1 {
		t.Fatal(month)
	}
}

func TestArchiveCanonicalPayloadKeepsSealedBytesAndRejectsStoredDamage(t *testing.T) {
	s, a := archiveFixture(t)
	r := archiveCreate(t, s, a, pastInput())
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	raw, hash, err := s.CanonicalPayloadTx(ctx, tx, testBrand, r.ID)
	if err != nil || len(raw) == 0 || hash != r.PayloadSHA256 {
		t.Fatal(hash, err)
	}
	if _, _, err = s.CanonicalPayloadTx(ctx, tx, otherBrand, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE report_archives DISABLE TRIGGER guarded_report_archive_version`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE report_archives SET payload_sha256=repeat('0',64) WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadTx(ctx, tx, testBrand, r.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatal("damaged snapshot read accepted", err)
	}
	if _, err = s.ListTx(ctx, tx, testBrand, 20, 0); !errors.Is(err, ErrIntegrity) {
		t.Fatal("damaged page accepted", err)
	}
	input := pastInput()
	input.ExpectedRevision = 1
	if _, err = s.CreateTx(ctx, tx, testBrand, a, input, archiveMeta(a)); !errors.Is(err, ErrIntegrity) {
		t.Fatal("damaged predecessor extended", err)
	}
	if raw, hash, err = s.CanonicalPayloadTx(ctx, tx, testBrand, r.ID); !errors.Is(err, ErrIntegrity) || raw != nil || hash != "" {
		t.Fatal("damaged download exposed", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := archiveRead(t, s, r.ID); !reflect.DeepEqual(got, r) {
		t.Fatal("rolled-back damage escaped owned schema transaction")
	}
}

func TestArchiveCaptureAndGuardShareSnapshotAcrossConcurrentFinancialCommit(t *testing.T) {
	s, a := archiveFixture(t)
	ctx := context.Background()
	// Keep the real capture implementation and inject only an owned-schema
	// barrier after it reads its one-statement source snapshot.
	if _, err := s.DB.Exec(ctx, `ALTER FUNCTION report_archive_capture(uuid,timestamptz,timestamptz,text) RENAME TO report_archive_capture_base;
 CREATE FUNCTION report_archive_capture(b uuid,f timestamptz,t timestamptz,z text DEFAULT NULL) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$DECLARE payload jsonb;BEGIN payload:=report_archive_capture_base(b,f,t,z);PERFORM pg_advisory_xact_lock(77266005);RETURN payload;END$$`); err != nil {
		t.Fatal(err)
	}
	barrier, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(ctx)
	if _, err = barrier.Exec(ctx, `SELECT pg_advisory_xact_lock(77266005)`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		r   Record
		err error
	}
	done := make(chan result, 1)
	go func() {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer tx.Rollback(ctx)
		r, err := s.CreateTx(ctx, tx, testBrand, a, pastInput(), archiveMeta(a))
		if err == nil {
			err = tx.Commit(ctx)
		}
		done <- result{r: r, err: err}
	}()
	var waiting bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks w JOIN pg_locks h USING(locktype,classid,objid,objsubid) WHERE w.locktype='advisory' AND NOT w.granted AND h.granted AND h.pid=$1 AND w.pid<>pg_backend_pid())`, barrier.Conn().PgConn().PID()).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("archive did not reach its source-snapshot barrier")
	}
	archiveFund(t, s) // A normal commit while archive capture remains live.
	if err = barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.r.Snapshot.WalletSnapshot.Balances.TotalPoints != "0" {
			t.Fatal("capture and source guard mixed snapshots", got.r, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("archive did not finish after capture barrier release")
	}
	var now string
	if err = s.DB.QueryRow(ctx, `SELECT sum(points)::text FROM point_buckets WHERE brand_id=$1`, testBrand).Scan(&now); err != nil || now != "13" {
		t.Fatal("concurrent genuine financial commit was lost", now, err)
	}
}

func TestArchiveRejectsStaleRevisionFuturePeriodsAndMissingPermissions(t *testing.T) {
	s, a := archiveFixture(t)
	in := pastInput()
	archiveCreate(t, s, a, in)
	ctx := context.Background()
	cases := []struct {
		actor access.Account
		input Input
		want  error
	}{
		{a, in, ErrVersion},
		{a, Input{Kind: Daily, PeriodKey: time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02"), Reason: "future"}, ErrState},
		{a, Input{Kind: Daily, PeriodKey: "2024-02-30", Reason: "invalid"}, ErrInvalid},
	}
	super := a
	super.SuperAdmin = true
	cases = append(cases, struct {
		actor access.Account
		input Input
		want  error
	}{super, in, ErrDenied})
	for _, tc := range cases {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.CreateTx(ctx, tx, testBrand, tc.actor, tc.input, archiveMeta(tc.actor))
		tx.Rollback(ctx)
		if !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
	}
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archives`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestArchiveCreationAuditFailureAndConcurrentRevisionAreAtomic(t *testing.T) {
	s, a := archiveFixture(t)
	ctx := context.Background()
	in := pastInput()
	if _, err := s.DB.Exec(ctx, `CREATE FUNCTION reject_archive_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action='report_archive.create' THEN RAISE EXCEPTION 'owned audit fault'; END IF; RETURN NEW; END$$; CREATE TRIGGER reject_archive_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_archive_audit()`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateTx(ctx, tx, testBrand, a, in, archiveMeta(a)); err == nil {
		t.Fatal("missing audit accepted")
	}
	tx.Rollback(ctx)
	var n int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archives`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err = s.DB.Exec(ctx, `DROP TRIGGER reject_archive_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(ctx)
			_, err = s.CreateTx(ctx, tx, testBrand, a, in, archiveMeta(a))
			if err == nil {
				err = tx.Commit(ctx)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	ok, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrBusy) || errors.Is(err, ErrVersion) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatal(ok, conflicts)
	}
}

func TestArchiveDatabaseRecomputesSourceSnapshotAndRejectsForgedContent(t *testing.T) {
	s, a := archiveFixture(t)
	in := pastInput()
	first := archiveCreate(t, s, a, in)
	ctx := context.Background()
	for _, fakeHash := range []bool{false, true} {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		id := ids.New()
		meta := archiveMeta(a)
		auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: testBrand, ActorType: "admin", ActorID: a.ID, Action: "report_archive.create", ResourceType: "report_archive", ResourceID: id, Reason: "owned forgery rejection", RequestID: meta.RequestID, Before: map[string]any{"id": first.ID, "revision": first.Revision, "payload_sha256": first.PayloadSHA256}, After: map[string]any{"id": id, "brand_id": testBrand, "kind": first.Window.Kind, "period_key": first.Window.PeriodKey, "timezone": first.Window.Timezone, "from": first.Window.From, "to": first.Window.To, "revision": 2, "previous_id": first.ID}})
		if err != nil {
			t.Fatal(err)
		}
		payloadExpr := `jsonb_set(v,'{wallet_snapshot,balances,total_points}','"999"'::jsonb)`
		hashExpr := `encode(sha256(convert_to(payload::text,'UTF8')),'hex')`
		if fakeHash {
			payloadExpr = "v"
			hashExpr = "repeat('0',64)"
		}
		_, err = tx.Exec(ctx, `WITH original AS MATERIALIZED (SELECT report_archive_capture($2,$6,$7,$5) v),capture AS MATERIALIZED (SELECT `+payloadExpr+` payload FROM original)
 INSERT INTO report_archives(id,brand_id,kind,period_key,timezone,from_at,to_at,revision,previous_id,snapshot_at,created_by,reason,request_id,payload,payload_sha256,audit_log_id,created_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,2,$8,(payload->>'snapshot_at')::timestamptz,$9,'owned forgery rejection',$10,payload,`+hashExpr+`,$11,(payload->>'snapshot_at')::timestamptz FROM capture`, id, testBrand, first.Window.Kind, first.Window.PeriodKey, first.Window.Timezone, first.Window.From, first.Window.To, first.ID, a.ID, meta.RequestID, auditID)
		if err == nil {
			t.Fatal("forged source snapshot or hash accepted")
		}
		tx.Rollback(ctx)
	}
	if got := archiveRead(t, s, first.ID); !reflect.DeepEqual(got, first) {
		t.Fatal("failed forgery changed archive")
	}
	// Go and PostgreSQL agree on normal DST days, repeated midnight and gaps.
	for _, tc := range []struct{ key, zone string }{{"2024-03-10", "America/New_York"}, {"2024-11-03", "America/New_York"}, {"2020-11-01", "America/Havana"}, {"2018-11-04", "America/Sao_Paulo"}} {
		w, err := ResolveWindow(Daily, tc.key, tc.zone)
		if err != nil {
			t.Fatal(err)
		}
		var from, to time.Time
		if err = s.DB.QueryRow(ctx, `SELECT report_archive_period_boundary($1::timestamp,$2),report_archive_period_boundary(($1::timestamp)+interval '1 day',$2)`, tc.key, tc.zone).Scan(&from, &to); err != nil || !from.Equal(w.From) || !to.Equal(w.To) {
			t.Fatal(w, from, to, err)
		}
	}
	var invalid *time.Time
	if err := s.DB.QueryRow(ctx, `SELECT report_archive_period_boundary('2011-12-30'::timestamp,'Pacific/Apia')`).Scan(&invalid); err != nil || invalid != nil {
		t.Fatal("wholly skipped civil date accepted", invalid, err)
	}
}
