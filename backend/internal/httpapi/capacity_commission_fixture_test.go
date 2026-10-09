//go:build capacity && !windows

package httpapi

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func capacityCommissionAgent(t *testing.T, p *pgxpool.Pool, brand, member string) string {
	t.Helper()
	ctx := context.Background()
	var brandID string
	if err := p.QueryRow(ctx, `SELECT id::text FROM brands WHERE code=$1`, brand).Scan(&brandID); err != nil {
		t.Fatalf("find capacity brand %s: %v", brand, err)
	}

	adminID, roleID := ids.New(), ids.New()
	permissionKeys := []string{
		"agent_policy.write.brand",
		"agent.write.brand",
		"join_code.write.brand",
	}
	capacityTx(t, p, func(tx pgx.Tx) error {
		var permissionCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE key=ANY($1::text[])`, permissionKeys).Scan(&permissionCount); err != nil {
			return err
		}
		if permissionCount != len(permissionKeys) {
			return fmt.Errorf("fixture database has %d of %d required agent permissions", permissionCount, len(permissionKeys))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'capacity-fixture-only-no-login')`, adminID, "capacity_agent_"+strings.ReplaceAll(adminID, "-", "")); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, adminID, brandID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO roles(id,code,name,brand_id) VALUES($1,$2,$3,$4)`, roleID, "capacity_agent_"+strings.ReplaceAll(roleID, "-", ""), "Capacity agent fixture operator", brandID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT $1,key FROM permissions WHERE key=ANY($2::text[])`, roleID, permissionKeys); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roleID)
		return err
	})

	actor, err := (adminsys.Store{DB: p}).Account(ctx, adminID)
	if err != nil {
		t.Fatalf("load persisted capacity agent operator for %s: %v", brand, err)
	}
	for _, grant := range []struct{ resource, action string }{
		{"agent_policy", "write"}, {"agent", "write"}, {"join_code", "write"},
	} {
		if !access.Authorize(actor, grant.resource, grant.action, access.ScopeBrand, brandID) {
			t.Fatalf("persisted capacity agent operator for %s lacks %s.%s.brand", brand, grant.resource, grant.action)
		}
	}

	agents := agency.Service{DB: p}
	codes := attribution.Service{DB: p}
	var policy agency.Policy
	capacityTx(t, p, func(tx pgx.Tx) error {
		var err error
		policy, err = agents.SavePolicy(ctx, tx, brandID, actor, agency.PolicyInput{
			Version: 1,
			Config:  agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "turnover", Cycle: "weekly"},
			Reason:  "capacity commission agent fixture policy",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})

	var root agency.Node
	capacityTx(t, p, func(tx pgx.Tx) error {
		var err error
		root, err = agents.Create(ctx, tx, brandID, actor, agency.CreateInput{
			PolicyVersion: policy.Version,
			MemberID:      member,
			Config:        agency.NodeConfig{Ratio: "0.1", Status: "active"},
			Reason:        "capacity commission agent fixture root",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})

	var code attribution.Code
	capacityTx(t, p, func(tx pgx.Tx) error {
		var err error
		code, err = codes.Create(ctx, tx, brandID, actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: member, AgentID: &root.ID, Reason: "capacity commission agent fixture join code",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	return code.Code
}

func TestCapacityCommissionAgentFixture(t *testing.T) {
	f := newCapacityFixture(t, 2, 2, 20)
	seenBrands := make(map[string]bool)
	for _, user := range f.Users {
		if seenBrands[user.Brand] {
			t.Fatalf("capacity fixture unexpectedly has multiple users for brand %s", user.Brand)
		}
		seenBrands[user.Brand] = true
		joinCode := capacityCommissionAgent(t, f.DB, user.Brand, user.Member)
		var brandID string
		if err := f.DB.QueryRow(context.Background(), `SELECT id::text FROM brands WHERE code=$1`, user.Brand).Scan(&brandID); err != nil {
			t.Fatalf("reload capacity brand %s: %v", user.Brand, err)
		}
		policy, err := (agency.Service{DB: f.DB}).Policy(context.Background(), brandID)
		if err != nil || !policy.Config.Enabled || policy.Config.MaxDepth != 3 || policy.Config.RatioCap != "0.1" || policy.Config.Mode != "turnover" || policy.Config.Cycle != "weekly" {
			t.Fatalf("capacity agent policy for %s was not persisted as requested: policy=%+v err=%v", user.Brand, policy, err)
		}
		node, err := (agency.Service{DB: f.DB}).Me(context.Background(), brandID, user.Member)
		if err != nil || node.Depth != 1 || node.Config.Ratio != "0.1" || node.Config.Status != "active" {
			t.Fatalf("capacity agent root for %s was not persisted as requested: node=%+v err=%v", user.Brand, node, err)
		}
		codes, err := (attribution.Service{DB: f.DB}).List(context.Background(), brandID, nil, &user.Member, 10, 0)
		if err != nil || len(codes.Items) != 1 || codes.Items[0].Code != joinCode || codes.Items[0].AgentID == nil || *codes.Items[0].AgentID != node.ID {
			t.Fatalf("capacity agent join code for %s was not persisted: codes=%+v err=%v", user.Brand, codes, err)
		}
	}
	if len(seenBrands) != 2 {
		t.Fatalf("capacity fixture covered %d brands, want 2", len(seenBrands))
	}
	var commissionCredits int
	if err := f.DB.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&commissionCredits); err != nil {
		t.Fatal("check commission credits after fixture setup:", err)
	}
	if commissionCredits != 0 {
		t.Fatalf("capacity agent fixture created %d commission credits", commissionCredits)
	}
}
