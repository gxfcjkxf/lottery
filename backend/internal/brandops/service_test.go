package brandops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const brandID = "0199a000-0000-7000-8000-000000000001"

func TestInputIsClosedAndValidated(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"status":"paused","reason":"maintenance"}`,
	} {
		var in Input
		if err := json.Unmarshal([]byte(raw), &in); err != nil || in.Version != 1 || in.Status != "paused" {
			t.Fatalf("valid input rejected: %+v, %v", in, err)
		}
	}
	for _, raw := range []string{
		`{"version":1,"status":"paused","reason":"ok","extra":true}`,
		`{"version":1,"version":2,"status":"paused","reason":"ok"}`,
		`{"version":null,"status":"paused","reason":"ok"}`,
		`{"version":1,"status":"paused","reason":null}`,
		`{"version":0,"status":"paused","reason":"ok"}`,
		`{"version":1,"status":"paused","reason":"  "}`,
		`{"version":1,"status":"paused","reason":"` + strings.Repeat("x", 501) + `"}`,
		`{"version":1,"status":"paused"}`,
	} {
		var in Input
		if err := json.Unmarshal([]byte(raw), &in); err == nil {
			t.Errorf("invalid input accepted: %s", raw)
		}
	}
}

func TestAllowedRequiresExactBrandOrPlatformGrant(t *testing.T) {
	brandGrant := access.Account{ID: "admin", Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}}}}}
	if !Allowed(brandGrant, brandID, "write") || Allowed(brandGrant, "0199a000-0000-7000-8000-000000000002", "write") {
		t.Fatal("brand grant must only authorize its own brand")
	}
	platformGrant := access.Account{ID: "admin", Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopePlatform}}}}}
	if !Allowed(platformGrant, brandID, "write") {
		t.Fatal("explicit platform grant should authorize")
	}
	if Allowed(access.Account{ID: "root", Type: access.AccountAdmin, SuperAdmin: true}, brandID, "write") {
		t.Fatal("superadmin must not imply brand-operation permissions")
	}
	platformGrant.Roles[0].Permissions = nil
	platformGrant.SuperAdmin = true
	if Allowed(platformGrant, brandID, "write") {
		t.Fatal("superadmin must not imply platform write")
	}
}

func TestUpdateWritesAtomicAuditAndImmutableHistory(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "brandops_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}, {Resource: "brand_operation", Action: "view", Scope: access.ScopeBrand}}}}}
	svc := Service{DB: p}
	var initial Record
	initial, err := svc.Read(ctx, brandID)
	if err != nil || initial.Version != 1 || initial.Status != "active" || initial.AuditLogID != "" {
		t.Fatalf("unexpected initial record: %+v err=%v", initial, err)
	}
	if rows, err := svc.History(ctx, brandID, 20, 0); err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("initial history should be empty and nonnil: %#v err=%v", rows, err)
	}
	meta := points.Metadata{ActorType: "admin", ActorID: adminID, Reason: "maintenance", IP: "127.0.0.1", RequestID: "brandops-test"}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Update(ctx, tx, brandID, actor, Input{Version: 1, Status: "paused", Reason: "maintenance"}, meta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if updated.Status != "paused" || updated.Version != 2 || updated.AuditLogID == "" || updated.Name != initial.Name {
		t.Fatalf("unexpected update record: %+v", updated)
	}
	record, err := svc.Read(ctx, brandID)
	if err != nil || record != updated {
		t.Fatalf("read mismatch record=%+v update=%+v err=%v", record, updated, err)
	}
	history, err := svc.History(ctx, brandID, 20, 0)
	if err != nil || len(history) != 1 || history[0].Version != 2 || history[0].PreviousStatus != "active" || history[0].Status != "paused" || history[0].ChangedBy != adminID || history[0].Reason != "maintenance" || history[0].AuditLogID != updated.AuditLogID {
		t.Fatalf("unexpected history: %+v err=%v", history, err)
	}
	var action, resource, reason, beforeStatus, afterStatus string
	var beforeVersion, afterVersion int64
	if err := p.QueryRow(ctx, `SELECT action,resource_type,reason,before_json->>'status',after_json->>'status',(before_json->>'version')::bigint,(after_json->>'version')::bigint FROM audit_logs WHERE id=$1`, updated.AuditLogID).Scan(&action, &resource, &reason, &beforeStatus, &afterStatus, &beforeVersion, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if action != "brand_operation.update" || resource != "brand_operation" || reason != "maintenance" || beforeStatus != "active" || afterStatus != "paused" || beforeVersion != 1 || afterVersion != 2 {
		t.Fatalf("audit witness mismatch: %s %s %s %s->%s %d->%d", action, resource, reason, beforeStatus, afterStatus, beforeVersion, afterVersion)
	}
	if _, err := p.Exec(ctx, `UPDATE brand_operation_revisions SET reason='tampered' WHERE brand_id=$1`, brandID); err == nil {
		t.Fatal("history update unexpectedly succeeded")
	}
	if _, err := p.Exec(ctx, `DELETE FROM brand_operation_revisions WHERE brand_id=$1`, brandID); err == nil {
		t.Fatal("history delete unexpectedly succeeded")
	}
}

func TestUpdateRejectsStateAndRollsBackWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "brandops_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}}}}}
	svc := Service{DB: p}
	meta := points.Metadata{ActorType: "admin", ActorID: adminID, Reason: "maintenance", RequestID: "brandops-rollback"}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Update(ctx, tx, brandID, actor, Input{Version: 1, Status: "paused", Reason: "maintenance"}, meta); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	record, err := svc.Read(ctx, brandID)
	if err != nil || record.Status != "active" || record.Version != 1 {
		t.Fatalf("rollback changed brand: %+v %v", record, err)
	}
	if n := count(t, p, `SELECT count(*) FROM brand_operation_revisions`); n != 0 {
		t.Fatalf("rollback left history: %d", n)
	}
	if n := count(t, p, `SELECT count(*) FROM audit_logs WHERE action='brand_operation.update'`); n != 0 {
		t.Fatalf("rollback left audit: %d", n)
	}
	for _, test := range []struct {
		in   Input
		want error
	}{{Input{Version: 1, Status: "active", Reason: "noop"}, ErrState}, {Input{Version: 1, Status: "disabled", Reason: "disable"}, ErrInvalid}} {
		tx, err = p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.Update(ctx, tx, brandID, actor, test.in, meta)
		_ = tx.Rollback(ctx)
		if !errors.Is(err, test.want) {
			t.Errorf("input %+v: got %v, want %v", test.in, err, test.want)
		}
	}
}

func TestUpdateSerializesVersionAndWaitsForUserBrandShareLock(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "brandops_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}}}}}
	svc := Service{DB: p}
	meta := points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: "brandops-concurrent"}
	first, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Update(ctx, first, brandID, actor, Input{Version: 1, Status: "paused", Reason: "first"}, meta); err != nil {
		_ = first.Rollback(ctx)
		t.Fatal(err)
	}
	second, err := p.Begin(ctx)
	if err != nil {
		_ = first.Rollback(ctx)
		t.Fatal(err)
	}
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		_, updateErr := svc.Update(ctx, second, brandID, actor, Input{Version: 1, Status: "paused", Reason: "second"}, meta)
		finished <- updateErr
	}()
	<-started
	if err = first.Commit(ctx); err != nil {
		_ = second.Rollback(ctx)
		t.Fatal(err)
	}
	select {
	case updateErr := <-finished:
		if !errors.Is(updateErr, ErrVersion) {
			_ = second.Rollback(ctx)
			t.Fatalf("second same-version update got %v, want ErrVersion", updateErr)
		}
	case <-time.After(5 * time.Second):
		_ = second.Rollback(ctx)
		t.Fatal("second same-version update remained blocked after first commit")
	}
	_ = second.Rollback(ctx)
	if n := count(t, p, `SELECT count(*) FROM brand_operation_revisions`); n != 1 {
		t.Fatalf("same-version writes produced %d revisions", n)
	}

	// A session-scoped read lock used by active user requests must drain before
	// the administrative state transition can acquire its FOR UPDATE lock.
	share, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = share.QueryRow(ctx, `SELECT id FROM brands WHERE id=$1 FOR SHARE`, brandID).Scan(new(string)); err != nil {
		_ = share.Rollback(ctx)
		t.Fatal(err)
	}
	updateTx, err := p.Begin(ctx)
	if err != nil {
		_ = share.Rollback(ctx)
		t.Fatal(err)
	}
	finished = make(chan error, 1)
	go func() {
		_, updateErr := svc.Update(ctx, updateTx, brandID, actor, Input{Version: 2, Status: "active", Reason: "resume"}, meta)
		finished <- updateErr
	}()
	select {
	case updateErr := <-finished:
		_ = share.Rollback(ctx)
		_ = updateTx.Rollback(ctx)
		t.Fatalf("update bypassed existing brand FOR SHARE lock: %v", updateErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err = share.Commit(ctx); err != nil {
		_ = updateTx.Rollback(ctx)
		t.Fatal(err)
	}
	select {
	case updateErr := <-finished:
		if updateErr != nil {
			_ = updateTx.Rollback(ctx)
			t.Fatalf("update after share-lock drain: %v", updateErr)
		}
	case <-time.After(5 * time.Second):
		_ = updateTx.Rollback(ctx)
		t.Fatal("update did not proceed after user brand lock drained")
	}
	if err = updateTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuditFailureRollsBackBrandAndHistory(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "brandops_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopeBrand}}}}}
	if _, err := p.Exec(ctx, `CREATE FUNCTION fail_brand_operation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='brand_operation.update' THEN RAISE EXCEPTION 'forced audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_brand_operation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_brand_operation_audit()`); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Service{DB: p}).Update(ctx, tx, brandID, actor, Input{Version: 1, Status: "paused", Reason: "maintenance"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: "brandops-audit-failure"})
	if err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("expected injected audit failure")
	}
	_ = tx.Rollback(ctx)
	var status string
	var version int64
	if err = p.QueryRow(ctx, `SELECT status,config_version FROM brands WHERE id=$1`, brandID).Scan(&status, &version); err != nil {
		t.Fatal(err)
	}
	if status != "active" || version != 1 || count(t, p, `SELECT count(*) FROM brand_operation_revisions`) != 0 || count(t, p, `SELECT count(*) FROM audit_logs WHERE action='brand_operation.update'`) != 0 {
		t.Fatalf("audit failure left partial state: status=%s version=%d", status, version)
	}
}

func TestHistoryWitnessRejectsNullAuditBrand(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID, auditID, revisionID := ids.New(), ids.New(), ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "brandops_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE brands SET status='paused',config_version=2,updated_at=clock_timestamp() WHERE id=$1`, brandID); err != nil {
		t.Fatal(err)
	}
	_, err := p.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,actor_id,action,resource_type,resource_id,reason,before_json,after_json,request_id)
		SELECT $1,NULL,'admin',$2,'brand_operation.update','brand_operation',b.id,'maintenance',
		jsonb_build_object('brand_id',b.id::text,'version',1,'name',b.name,'status','active'),
		jsonb_build_object('brand_id',b.id::text,'version',b.config_version,'name',b.name,'status',b.status,'updated_at',b.updated_at),'null-brand-test'
		FROM brands b WHERE b.id=$3`, auditID, adminID, brandID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO brand_operation_revisions(id,brand_id,version,previous_status,status,changed_by,reason,audit_log_id)
		VALUES($1,$2,2,'active','paused',$3,'maintenance',$4)`, revisionID, brandID, adminID, auditID); err == nil {
		t.Fatal("history accepted an audit witness with NULL brand_id")
	}
	if n := count(t, p, `SELECT count(*) FROM brand_operation_revisions`); n != 0 {
		t.Fatalf("forged history row was inserted: %d", n)
	}
}

func count(t *testing.T, p interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, query string) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
