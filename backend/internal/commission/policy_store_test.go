package commission

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestScanPolicyRequiresMatchingRevisionEvidence(t *testing.T) {
	config := DefaultPolicyConfig()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	id, auditID := "0199a000-0000-7000-8000-000000000001", "0199a000-0000-7000-8000-000000000002"
	base := []any{"0199a000-0000-7000-8000-000000000003", int64(1), raw, stamp, stamp, &id, &auditID, raw}
	if got, err := scanPolicy(valuesRow(base)); err != nil || got.RevisionID != id {
		t.Fatalf("scanPolicy() = %+v, %v", got, err)
	}
	missing := append([]any(nil), base...)
	missing[5] = (*string)(nil)
	if _, err := scanPolicy(valuesRow(missing)); !errors.Is(err, ErrPolicyEvidence) {
		t.Fatalf("missing revision evidence error = %v", err)
	}
	other := PolicyConfig{PayoutMode: PayoutAutomatic}
	otherRaw, _ := json.Marshal(other)
	mismatched := append([]any(nil), base...)
	mismatched[7] = otherRaw
	if _, err := scanPolicy(valuesRow(mismatched)); !errors.Is(err, ErrPolicyEvidence) {
		t.Fatalf("mismatched revision config error = %v", err)
	}
}

type valuesRow []any

func (values valuesRow) Scan(dest ...any) error {
	if len(values) != len(dest) {
		return errors.New("scan destination count mismatch")
	}
	for i, value := range values {
		switch target := dest[i].(type) {
		case *string:
			if value == nil {
				return errors.New("unexpected null string")
			}
			*target = value.(string)
		case **string:
			*target = value.(*string)
		case *int64:
			*target = value.(int64)
		case *[]byte:
			*target = value.([]byte)
		case *time.Time:
			*target = value.(time.Time)
		default:
			return errors.New("unsupported scan destination")
		}
	}
	return nil
}

func TestPolicyStoreRevisionAuditEvidenceAndSQLGuards(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var ready bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('brand_commission_policies') IS NOT NULL AND to_regclass('commission_policy_revisions') IS NOT NULL`).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("migration 047 did not create commission policy tables")
	}
	var brand string
	if err := db.QueryRow(ctx, `SELECT id::text FROM brands ORDER BY id LIMIT 1`).Scan(&brand); err != nil {
		t.Fatal(err)
	}
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "commission_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{
		{Resource: "commission_policy", Action: "write", Scope: access.ScopeBrand},
		{Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand},
	}}}}
	service := Service{DB: db}
	agencyService := agency.Service{DB: db}
	initial, err := service.Policy(ctx, brand)
	if err != nil || initial.Version != 1 || !reflect.DeepEqual(initial.Config, DefaultPolicyConfig()) || initial.RevisionID == "" || initial.AuditLogID != "" {
		t.Fatalf("initial commission policy = %+v, err=%v", initial, err)
	}
	agencyPolicy, err := agencyService.Policy(ctx, brand)
	if err != nil || agencyPolicy.Version != 1 {
		t.Fatalf("initial agency policy = %+v, err=%v", agencyPolicy, err)
	}
	agencyConfig := agency.PolicyConfig{Enabled: true, MaxDepth: 5, RatioCap: "0", Mode: "loss", Cycle: "weekly"}
	agencyMeta := points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
	if err = agencyUpdateTest(t, agencyService, brand, actor, agency.PolicyInput{Version: agencyPolicy.Version, Config: agencyConfig, Reason: "enable weekly agency policy fixture"}, agencyMeta); err != nil {
		t.Fatalf("enable agency policy through revision service: %v", err)
	}
	agencyPolicy, err = agencyService.Policy(ctx, brand)
	if err != nil || !agencyPolicy.Config.Enabled || agencyPolicy.Config.Cycle != "weekly" || agencyPolicy.Version != 2 {
		t.Fatalf("enabled agency policy = %+v, err=%v", agencyPolicy, err)
	}

	weekly := PolicyConfig{Enabled: true, Calendar: &Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: intPointer(1)}, PayoutMode: PayoutManual}
	meta := points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
	if _, err = updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: initial.Version, Config: weekly, Reason: "enable weekly commission cycle"}, meta); err != nil {
		t.Fatalf("enable commission policy with matching agency evidence: %v", err)
	}
	updated, err := service.Policy(ctx, brand)
	if err != nil || updated.Version != 2 || !updated.Config.Enabled || updated.RevisionID == initial.RevisionID || updated.AuditLogID == "" {
		t.Fatalf("updated commission policy = %+v, err=%v", updated, err)
	}
	betTx, err := service.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = LockBetPolicies(ctx, betTx, brand); err != nil {
		_ = betTx.Rollback(ctx)
		t.Fatalf("LockBetPolicies() with enabled matching evidence: %v", err)
	}
	if err = betTx.Commit(ctx); err != nil {
		t.Fatalf("commit betting policy evidence locks: %v", err)
	}

	monthlyAgency := agencyConfig
	monthlyAgency.Cycle = "monthly"
	if err = agencyUpdateTest(t, agencyService, brand, actor, agency.PolicyInput{Version: agencyPolicy.Version, Config: monthlyAgency, Reason: "reject agency cycle drift"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); !errors.Is(err, agency.ErrState) {
		t.Fatalf("agency service cycle-change error = %v, want agency.ErrState", err)
	}
	if _, err = db.Exec(ctx, `UPDATE brand_agent_policies SET version=version+1,config=jsonb_set(config,'{cycle}','"monthly"'::jsonb) WHERE brand_id=$1`, brand); err == nil {
		t.Fatal("direct SQL agency cycle change bypassed enabled commission policy")
	}
	agencyAfterReject, err := agencyService.Policy(ctx, brand)
	if err != nil || agencyAfterReject.Version != agencyPolicy.Version || agencyAfterReject.Config.Cycle != "weekly" {
		t.Fatalf("rejected agency cycle change mutated policy: %+v, err=%v", agencyAfterReject, err)
	}

	monthly := PolicyConfig{Enabled: true, Calendar: &Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: intPointer(1), ShortMonth: "last_day"}, PayoutMode: PayoutManual}
	if _, err = updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: updated.Version, Config: monthly, Reason: "reject mismatched cycle"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); !errors.Is(err, ErrPolicyEvidence) {
		t.Fatalf("mismatched agency cycle error = %v", err)
	}
	superAdmin := actor
	superAdmin.SuperAdmin = true
	if _, err = updatePolicyValue(ctx, service, brand, superAdmin, PolicyInput{Version: updated.Version, Config: monthly, Reason: "superadmin must not write"}, points.Metadata{ActorType: "admin", ActorID: superAdmin.ID, RequestID: ids.New()}); !errors.Is(err, ErrDenied) {
		t.Fatalf("superadmin write error = %v", err)
	}
	if _, err = updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: initial.Version, Config: weekly, Reason: "stale version"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale commission version error = %v", err)
	}

	history, err := service.History(ctx, brand, 10, 0)
	if err != nil || len(history) != 2 || history[0].ID != updated.RevisionID || history[1].ID != initial.RevisionID || history[1].ChangedBy != nil || history[1].AuditLogID != nil {
		t.Fatalf("commission history = %+v, err=%v", history, err)
	}
	if _, err = service.History(ctx, brand, 1, 1); err != nil {
		t.Fatalf("commission history page: %v", err)
	}
	assertPolicySQLGuards(t, service, brand, initial.RevisionID)
	assertPolicyAuditFailureRollsBack(t, service, brand, actor, updated)
}

func intPointer(value int) *int { return &value }

func agencyUpdateTest(t *testing.T, service agency.Service, brand string, actor access.Account, input agency.PolicyInput, meta points.Metadata) error {
	t.Helper()
	tx, err := service.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = service.SavePolicy(context.Background(), tx, brand, actor, input, meta); err != nil {
		return err
	}
	return tx.Commit(context.Background())
}

func updatePolicyValue(ctx context.Context, service Service, brand string, actor access.Account, input PolicyInput, meta points.Metadata) (Policy, error) {
	tx, err := service.DB.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	updated, err := service.Update(ctx, tx, brand, actor, input, meta)
	if err != nil {
		return Policy{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return updated, nil
}

func assertPolicySQLGuards(t *testing.T, service Service, brand, initialRevision string) {
	t.Helper()
	ctx := context.Background()
	tx, err := service.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := tx.Exec(ctx, `UPDATE brand_commission_policies SET version=version+1,config=$2 WHERE brand_id=$1`, brand, []byte(`{"enabled":false,"calendar":null,"payout_mode":"automatic"}`))
	commitErr := tx.Commit(ctx)
	if execErr == nil && commitErr == nil {
		t.Fatal("direct commission policy update without revision committed")
	}
	for _, statement := range []string{
		`UPDATE commission_policy_revisions SET reason='tampered' WHERE id=$1`,
		`DELETE FROM commission_policy_revisions WHERE id=$1`,
	} {
		if _, err = service.DB.Exec(ctx, statement, initialRevision); err == nil {
			t.Fatalf("direct SQL mutation unexpectedly succeeded: %s", statement)
		}
	}
}

func assertPolicyAuditFailureRollsBack(t *testing.T, service Service, brand string, actor access.Account, current Policy) {
	t.Helper()
	ctx := context.Background()
	if _, err := service.DB.Exec(ctx, `CREATE FUNCTION reject_commission_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.action='commission.policy.update' THEN RAISE EXCEPTION 'injected commission audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DB.Exec(ctx, `CREATE TRIGGER reject_commission_policy_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_commission_policy_audit()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = service.DB.Exec(ctx, `DROP TRIGGER IF EXISTS reject_commission_policy_audit ON audit_logs`)
		_, _ = service.DB.Exec(ctx, `DROP FUNCTION IF EXISTS reject_commission_policy_audit()`)
	}()
	var revisionsBefore, auditsBefore int
	if err := service.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM commission_policy_revisions WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.policy.update')`, brand).Scan(&revisionsBefore, &auditsBefore); err != nil {
		t.Fatal(err)
	}
	config := current.Config
	config.PayoutMode = PayoutAutomatic
	if _, err := updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: current.Version, Config: config, Reason: "injected audit failure"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); err == nil {
		t.Fatal("policy update succeeded despite audit insertion failure")
	}
	after, err := service.Policy(ctx, brand)
	if err != nil || after.Version != current.Version || after.RevisionID != current.RevisionID || after.Config.PayoutMode != current.Config.PayoutMode {
		t.Fatalf("audit failure leaked policy mutation: %+v, err=%v", after, err)
	}
	var revisionsAfter, auditsAfter int
	if err = service.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM commission_policy_revisions WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.policy.update')`, brand).Scan(&revisionsAfter, &auditsAfter); err != nil {
		t.Fatal(err)
	}
	if revisionsAfter != revisionsBefore || auditsAfter != auditsBefore {
		t.Fatalf("audit failure leaked records: revisions %d->%d, audits %d->%d", revisionsBefore, revisionsAfter, auditsBefore, auditsAfter)
	}
}
