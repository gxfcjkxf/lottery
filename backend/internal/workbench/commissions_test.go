package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCommissionWorkbenchIndependentViewActualZerosAndNoUnauthorizedReads(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	out, err := snapshot(t, db, actor("commission"), brandA)
	if err != nil || out.Commissions.Status != "ready" || out.Commissions.Data == nil {
		t.Fatal("independent commission view", out.Commissions, err)
	}
	if out.Brand.Status != "forbidden" || out.Ledger.Status != "forbidden" || out.Balances.Data != nil {
		t.Fatal("task view leaked wallet/other sections")
	}
	raw, _ := json.Marshal(out.Commissions.Data)
	var values map[string]string
	if err = json.Unmarshal(raw, &values); err != nil || len(values) != 19 {
		t.Fatal("closed count fields", string(raw), err)
	}
	for key, value := range values {
		if value != "0" {
			t.Fatal("actual empty task count", key, value)
		}
	}
	platform := access.Account{Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopePlatform}}}}}
	if out, err = snapshot(t, db, platform, brandB); err != nil || out.Commissions.Status != "ready" {
		t.Fatal("platform view", out.Commissions, err)
	}
	for _, resource := range []string{"wallet", "report_commission", "brand"} {
		out, err = snapshot(t, db, actor(resource), brandA)
		if resource == "report_commission" {
			if !errors.Is(err, ErrDenied) {
				t.Fatal("posting grant alone implies task view", err)
			}
			continue
		}
		if err != nil || out.Commissions.Status != "forbidden" || out.Commissions.Data != nil {
			t.Fatal("wrong resource granted commission data", resource, out.Commissions, err)
		}
	}
	for _, table := range []string{"commission_discovery", "commission_cycles", "commission_payments", "commission_correction_plans", "commission_correction_executions"} {
		if _, err = db.Exec(ctx, `ALTER TABLE `+table+` RENAME TO workbench_hidden_`+table); err != nil {
			t.Fatal(err)
		}
	}
	out, err = snapshot(t, db, actor("brand"), brandA)
	if err != nil || out.Commissions.Status != "forbidden" || out.Commissions.Data != nil {
		t.Fatal("forbidden fragment accessed task tables", out.Commissions, err)
	}
	if _, err = snapshot(t, db, actor("commission"), brandA); err == nil {
		t.Fatal("broken authorized source reported zero tasks")
	}
}
