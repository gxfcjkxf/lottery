package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"testing"
)

func TestPlatformCommissionInheritanceAndPermissions(t *testing.T) {
	f := managedFixture(t)
	op := securityOperator(t, f)
	path := "/api/v1/platform/commission-settings"
	mustStatus(t, op.call("GET", path, "", op.token, "", nil), 403)
	role := grantPlatformPermission(t, op, "platform_settings.view.platform", "platform_settings.write.platform")
	r := op.call("GET", path, "", op.token, "", nil)
	mustStatus(t, r, 200)
	var initial commission.PlatformPolicy
	managedData(t, r, &initial)
	var timezone string
	if err := f.pool.QueryRow(context.Background(), `SELECT timezone FROM platform_settings WHERE id=true`).Scan(&timezone); err != nil {
		t.Fatal(err)
	}
	day := 1
	in := commission.PlatformPolicyInput{PayoutsEnabled: true, Version: initial.Version, Config: commission.PolicyConfig{Enabled: true, PayoutMode: "automatic", Calendar: &commission.Calendar{Timezone: timezone, Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &day}}}
	mustStatus(t, f.call("PUT", "/api/v1/admin/commission-policy", "brand-denied", f.token, managedBrand, in), 403)
	first := op.call("PUT", path, "commission-platform-save", op.token, "", in)
	mustStatus(t, first, 200)
	replay := op.call("PUT", path, "commission-platform-save", op.token, "", in)
	mustStatus(t, replay, 200)
	var saved, replayed commission.PlatformPolicy
	managedData(t, first, &saved)
	managedData(t, replay, &replayed)
	if saved.Version != initial.Version+1 || replayed.Version != saved.Version {
		t.Fatal(saved, replayed)
	}
	var auditCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='commission.platform_policy.update'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("replay duplicated platform audit", auditCount, err)
	}
	var mismatches int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM brand_commission_policies WHERE config<>(SELECT config FROM platform_commission_policy WHERE id=true)`).Scan(&mismatches); err != nil || mismatches != 0 {
		t.Fatal(mismatches, err)
	}
	if !saved.PayoutsEnabled {
		t.Fatal("platform payout gate was not enabled")
	}
	newBrand := ids.New()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Commission test only','paused')`, newBrand, "commission_"+newBrand); err != nil {
		t.Fatal(err)
	}
	var gatesEnabled bool
	if err := f.pool.QueryRow(context.Background(), `SELECT bool_and(enabled) FROM brand_commission_payment_policies`).Scan(&gatesEnabled); err != nil || !gatesEnabled {
		t.Fatal("brands did not inherit payouts", err)
	}
	mustStatus(t, f.call("PUT", commissionPaymentPolicyHTTPPath, "brand-payout-denied", f.token, managedBrand, map[string]any{"version": 1, "enabled": false}), 403)
	var inherited bool
	if err := f.pool.QueryRow(context.Background(), `SELECT config=(SELECT config FROM platform_commission_policy WHERE id=true) FROM brand_commission_policies WHERE brand_id=$1`, newBrand).Scan(&inherited); err != nil || !inherited {
		t.Fatal("new brand did not inherit", err)
	}

	// Failure while propagating a gate must roll back the platform row, brand
	// gates and receipts together; retrying the same key must remain possible.
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_payout_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.payment_policy.update' THEN RAISE EXCEPTION 'test propagation failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_payout_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_payout_audit()`); err != nil {
		t.Fatal(err)
	}
	in.Version = saved.Version
	in.PayoutsEnabled = false
	mustStatus(t, op.call("PUT", path, "platform-disable-retry", op.token, "", in), 503)
	var rolledBack commission.PlatformPolicy
	read := op.call("GET", path, "", op.token, "", nil)
	mustStatus(t, read, 200)
	managedData(t, read, &rolledBack)
	if !rolledBack.PayoutsEnabled || rolledBack.Version != saved.Version {
		t.Fatal("failed propagation changed platform gate", rolledBack)
	}
	if _, err := f.pool.Exec(context.Background(), `DROP TRIGGER reject_payout_audit ON audit_logs; DROP FUNCTION reject_payout_audit()`); err != nil {
		t.Fatal(err)
	}
	disabled := op.call("PUT", path, "platform-disable-retry", op.token, "", in)
	mustStatus(t, disabled, 200)
	var disabledResult commission.PlatformPolicy
	managedData(t, disabled, &disabledResult)
	if disabledResult.PayoutsEnabled || disabledResult.Version != saved.Version+1 {
		t.Fatal(disabledResult)
	}
	var enabledCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM brand_commission_payment_policies WHERE enabled`).Scan(&enabledCount); err != nil || enabledCount != 0 {
		t.Fatal("disable did not propagate", enabledCount, err)
	}
	changed := in
	changed.PayoutsEnabled = true
	mustStatus(t, op.call("PUT", path, "platform-disable-retry", op.token, "", changed), 409)
	if _, err := f.pool.Exec(context.Background(), `UPDATE platform_settings SET timezone='Asia/Tokyo' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	var wrongZones int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM brand_commission_policies WHERE config->'calendar'->>'timezone'<>'Asia/Tokyo' OR config->'calendar'->>'boundary_time'<>'00:00:00'`).Scan(&wrongZones); err != nil || wrongZones != 0 {
		t.Fatal(wrongZones, err)
	}
	in.Config.Calendar.Timezone = "Asia/Tokyo"
	mustStatus(t, op.call("PUT", path, "commission-stale", op.token, "", in), 409)
	in.Version = saved.Version
	in.Config.Calendar.BoundaryTime = "12:00:00"
	mustStatus(t, op.call("PUT", path, "commission-nonmidnight", op.token, "", in), 400)
	in.Config.Calendar.BoundaryTime = "00:00:00"
	in.Config.Calendar.Timezone = "Pacific/Kiritimati"
	mustStatus(t, op.call("PUT", path, "commission-wrong-timezone", op.token, "", in), 400)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_settings.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, op.call("PUT", path, "commission-platform-save", op.token, "", in), 403)
}
