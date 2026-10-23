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

func TestScanPolicyUsesCurrentConfiguration(t *testing.T) {
	raw, _ := json.Marshal(DefaultPolicyConfig())
	stamp := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	id := "0199a000-0000-7000-8000-000000000001"
	base := []any{"0199a000-0000-7000-8000-000000000003", int64(1), raw, stamp, stamp, id}
	if got, err := scanPolicy(valuesRow(base)); err != nil || got.RevisionID != id {
		t.Fatalf("current policy: %+v %v", got, err)
	}
	base[5] = "invalid"
	if _, err := scanPolicy(valuesRow(base)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid current token: %v", err)
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

func TestPolicyStoreCurrentOnlyAndSQLGuards(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var ready bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('brand_commission_policies') IS NOT NULL AND to_regclass('commission_policy_revisions') IS NULL`).Scan(&ready); err != nil {
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
	if err != nil || initial.Version != 1 || !reflect.DeepEqual(initial.Config, DefaultPolicyConfig()) || initial.RevisionID == "" {
		t.Fatalf("initial commission policy = %+v, err=%v", initial, err)
	}
	agencyPolicy, err := agencyService.Policy(ctx, brand)
	if err != nil || agencyPolicy.Version != 1 {
		t.Fatalf("initial agency policy = %+v, err=%v", agencyPolicy, err)
	}
	agencyConfig := agency.PolicyConfig{Enabled: true, MaxDepth: 5, LossRatioCap: "0", TurnoverRatioCap: "0", Mode: "loss", Cycle: "weekly"}
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
	if err != nil || updated.Version != 2 || !updated.Config.Enabled || updated.RevisionID == initial.RevisionID {
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
	monthlySaved, err := updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: updated.Version, Config: monthly}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
	if err != nil {
		t.Fatalf("single calendar update: %v", err)
	}
	synced, err := agencyService.Policy(ctx, brand)
	if err != nil || synced.Config.Cycle != "monthly" || synced.Version != agencyPolicy.Version+1 {
		t.Fatalf("agency snapshot cycle did not synchronize: %+v %v", synced, err)
	}
	if _, err = db.Exec(ctx, `CREATE FUNCTION reject_cycle_sync() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.config->'cycle' IS DISTINCT FROM OLD.config->'cycle' THEN RAISE EXCEPTION 'injected sync failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_cycle_sync BEFORE UPDATE ON brand_agent_policies FOR EACH ROW EXECUTE FUNCTION reject_cycle_sync()`); err != nil {
		t.Fatal(err)
	}
	if _, err = updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: monthlySaved.Version, Config: weekly}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); err == nil {
		t.Fatal("calendar save succeeded despite cycle synchronization failure")
	}
	unchanged, err := service.Policy(ctx, brand)
	if err != nil || unchanged.Version != monthlySaved.Version || unchanged.Config.Calendar.Cycle != "monthly" {
		t.Fatalf("failed synchronization leaked financial policy: %+v %v", unchanged, err)
	}
	if _, err = db.Exec(ctx, `DROP TRIGGER reject_cycle_sync ON brand_agent_policies; DROP FUNCTION reject_cycle_sync()`); err != nil {
		t.Fatal(err)
	}

	superAdmin := actor
	superAdmin.SuperAdmin = true
	if _, err = updatePolicyValue(ctx, service, brand, superAdmin, PolicyInput{Version: updated.Version, Config: monthly, Reason: "superadmin must not write"}, points.Metadata{ActorType: "admin", ActorID: superAdmin.ID, RequestID: ids.New()}); !errors.Is(err, ErrDenied) {
		t.Fatalf("superadmin write error = %v", err)
	}
	if _, err = updatePolicyValue(ctx, service, brand, actor, PolicyInput{Version: initial.Version, Config: weekly, Reason: "stale version"}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale commission version error = %v", err)
	}

	assertPolicySQLGuards(t, service, brand)
	var policyAudits, agencyHistory int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action IN ('commission.policy.update','agent.policy.update')),(SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NULL)`, brand).Scan(&policyAudits, &agencyHistory); err != nil || policyAudits != 0 || agencyHistory != 0 {
		t.Fatalf("policy history recorded: %d %d %v", policyAudits, agencyHistory, err)
	}

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

func assertPolicySQLGuards(t *testing.T, service Service, brand string) {
	t.Helper()
	for _, statement := range []string{
		`UPDATE brand_commission_policies SET version=version+2 WHERE brand_id=$1`,
		`UPDATE brand_commission_policies SET config='{}'::jsonb,version=version+1 WHERE brand_id=$1`,
		`DELETE FROM brand_commission_policies WHERE brand_id=$1`,
	} {
		if _, err := service.DB.Exec(context.Background(), statement, brand); err == nil {
			t.Fatalf("invalid SQL mutation succeeded: %s", statement)
		}
	}
}
