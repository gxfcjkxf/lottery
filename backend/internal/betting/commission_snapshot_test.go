package betting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestBetCommissionRuleSnapshotCapturesPrivateImmutablePlacementEvidence(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	var ready bool
	if err := f.db.QueryRow(ctx, `SELECT
 to_regclass('brand_commission_policies') IS NOT NULL
 AND to_regclass('commission_policy_revisions') IS NOT NULL
 AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='bet_orders' AND column_name='commission_rule_snapshot')`).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("commission rule snapshot schema is required after migration 047")
	}

	admin := f.version.CreatedBy
	actor := access.Account{
		ID: admin, Type: access.AccountAdmin, BrandIDs: []string{f.brand},
		Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{
			{Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "agent", Action: "write", Scope: access.ScopeBrand},
			{Resource: "join_code", Action: "write", Scope: access.ScopeBrand},
			{Resource: "commission_policy", Action: "write", Scope: access.ScopeBrand},
		}}},
	}
	agents := agency.Service{DB: f.db}
	financial := commission.Service{DB: f.db}
	codes := attribution.Service{DB: f.db}
	var agentPolicy agency.Policy
	var node agency.Node
	var code attribution.Code
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		agentPolicy, err = agents.SavePolicy(ctx, tx, f.brand, actor, agency.PolicyInput{
			Version: 1,
			Config:  agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "loss", Cycle: "weekly"},
			Reason:  "commission snapshot fixture evidence",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		node, err = agents.Create(ctx, tx, f.brand, actor, agency.CreateInput{
			PolicyVersion: agentPolicy.Version, MemberID: f.member,
			Config: agency.NodeConfig{Ratio: "0.08", Status: "active", CanCreateChildren: true},
			Reason: "commission snapshot fixture agent",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		code, err = codes.Create(ctx, tx, f.brand, actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: f.member, AgentID: &node.ID, Reason: "commission snapshot fixture join code",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})

	users, err := identity.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	var session identity.Authentication
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		out, err := users.Register(ctx, tx, f.brand, identity.RegisterInput{
			Username: "bet_commission_" + strings.ReplaceAll(ids.New(), "-", "")[:12],
			Password: "test-commission-snapshot-password", Privacy: "dev-1", Terms: "dev-1", AgentCode: code.Code,
		}, identity.Metadata{Domain: "aurora.localhost"})
		if err == nil {
			if out.Status != 201 {
				t.Fatalf("register status=%d", out.Status)
			}
			err = json.Unmarshal(out.Data, &session)
		}
		return err
	})
	f.user, err = users.Authenticate(ctx, f.brand, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	f.member, f.token = session.Member.ID, session.AccessToken
	fundBettingWallet(t, f, 20)

	initialPolicy, err := financial.Policy(ctx, f.brand)
	if err != nil || initialPolicy.Version != 1 || initialPolicy.Config != commission.DefaultPolicyConfig() || initialPolicy.RevisionID == "" {
		t.Fatalf("initial financial policy=%+v err=%v", initialPolicy, err)
	}
	first, err := placeBettingOrder(t, f, f.input, "commission-snapshot-first")
	if err != nil {
		t.Fatal(err)
	}
	read := func(column, id string) []byte {
		t.Helper()
		var raw []byte
		query := `SELECT ` + column + ` FROM bet_orders WHERE id=$1`
		if err := f.db.QueryRow(ctx, query, id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	readPolicySnapshot := func(brand, order string) (commission.BetSnapshot, error) {
		tx, err := f.db.Begin(ctx)
		if err != nil {
			return commission.BetSnapshot{}, err
		}
		defer tx.Rollback(ctx)
		return (commission.Source{}).PolicySnapshotTx(ctx, tx, brand, order)
	}
	firstSnapshot := read("commission_rule_snapshot", first.ID)
	firstAttribution := read("attribution_snapshot", first.ID)
	assertCommissionSnapshot(t, firstSnapshot, f.brand, f.member, first.PlacedAt, initialPolicy.RevisionID, "1", false, node, "1", "0.08", "active")
	assertAttributionCommissionNull(t, firstAttribution)
	firstEvidence, err := readPolicySnapshot(f.brand, first.ID)
	if err != nil {
		t.Fatalf("read first commission policy evidence: %v", err)
	}
	firstEvidenceJSON, err := json.Marshal(firstEvidence)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionSnapshot(t, firstEvidenceJSON, f.brand, f.member, first.PlacedAt, initialPolicy.RevisionID, "1", false, node, "1", "0.08", "active")
	if _, err = readPolicySnapshot(storeTestOtherBrand, first.ID); !errors.Is(err, commission.ErrNotFound) {
		t.Fatalf("foreign-brand snapshot read error=%v, want not found", err)
	}

	updatedNodeConfig := node.Config
	updatedNodeConfig.Ratio = "0.06"
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		node, err = agents.Update(ctx, tx, f.brand, actor, node.ID, agency.UpdateInput{
			Version: node.Version, PolicyVersion: agentPolicy.Version, Config: updatedNodeConfig,
			Reason: "new stakes use updated agent ratio",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	weekday := 1
	updatedFinancial, err := updateCommissionSnapshotPolicy(ctx, f.db, financial, f.brand, actor, commission.PolicyInput{
		Version: initialPolicy.Version,
		Config: commission.PolicyConfig{
			Enabled:    true,
			Calendar:   &commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &weekday},
			PayoutMode: commission.PayoutManual,
		},
		Reason: "enable weekly commission snapshot policy",
	})
	if err != nil || updatedFinancial.Version != 2 || updatedFinancial.RevisionID == initialPolicy.RevisionID {
		t.Fatalf("updated financial policy=%+v err=%v", updatedFinancial, err)
	}

	second, err := placeBettingOrder(t, f, f.input, "commission-snapshot-second")
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot := read("commission_rule_snapshot", second.ID)
	assertCommissionSnapshot(t, secondSnapshot, f.brand, f.member, second.PlacedAt, updatedFinancial.RevisionID, "2", true, node, "2", "0.06", "active")
	assertAttributionCommissionNull(t, read("attribution_snapshot", second.ID))
	secondEvidence, err := readPolicySnapshot(f.brand, second.ID)
	if err != nil {
		t.Fatalf("read second commission policy evidence: %v", err)
	}
	if secondEvidence.Financial.Version != "2" || secondEvidence.Path[0].Config.Ratio != "0.06" {
		t.Fatalf("second read evidence=%+v", secondEvidence)
	}
	if !bytes.Equal(read("commission_rule_snapshot", first.ID), firstSnapshot) {
		t.Fatal("later policy or ratio edits rewrote the first commission snapshot")
	}
	if !bytes.Equal(read("attribution_snapshot", first.ID), firstAttribution) {
		t.Fatal("commission snapshot capture rewrote the existing attribution snapshot")
	}

	disabledNodeConfig := node.Config
	disabledNodeConfig.Status = "disabled"
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		node, err = agents.Update(ctx, tx, f.brand, actor, node.ID, agency.UpdateInput{
			Version: node.Version, PolicyVersion: agentPolicy.Version, Config: disabledNodeConfig,
			Reason: "disable agent operations while retaining commission evidence",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	third, err := placeBettingOrder(t, f, f.input, "commission-snapshot-disabled-agent")
	if err != nil {
		t.Fatalf("placing a bet for a member attributed to a disabled agent: %v", err)
	}
	thirdSnapshot := read("commission_rule_snapshot", third.ID)
	assertCommissionSnapshot(t, thirdSnapshot, f.brand, f.member, third.PlacedAt, updatedFinancial.RevisionID, "2", true, node, "3", "0.06", "disabled")
	assertAttributionCommissionNull(t, read("attribution_snapshot", third.ID))
	thirdEvidence, err := readPolicySnapshot(f.brand, third.ID)
	if err != nil || thirdEvidence.Path[0].Config.Status != "disabled" || thirdEvidence.Path[0].EffectiveMode != "loss" {
		t.Fatalf("read disabled-agent commission evidence=%+v err=%v", thirdEvidence, err)
	}
	firstEvidenceAfterEdits, err := readPolicySnapshot(f.brand, first.ID)
	if err != nil || !reflect.DeepEqual(firstEvidenceAfterEdits, firstEvidence) {
		t.Fatalf("historical commission evidence changed after policy/ratio/status edits: before=%+v after=%+v err=%v", firstEvidence, firstEvidenceAfterEdits, err)
	}
	secondEvidenceAfterDisable, err := readPolicySnapshot(f.brand, second.ID)
	if err != nil || !reflect.DeepEqual(secondEvidenceAfterDisable, secondEvidence) {
		t.Fatalf("second bet evidence changed after agent disable: before=%+v after=%+v err=%v", secondEvidence, secondEvidenceAfterDisable, err)
	}
	if !bytes.Equal(read("commission_rule_snapshot", first.ID), firstSnapshot) || !bytes.Equal(read("commission_rule_snapshot", second.ID), secondSnapshot) {
		t.Fatal("disabling the agent rewrote a previously captured commission snapshot")
	}

	walletBeforeReplay := walletBySource(t, f)
	replay, err := placeBettingOrder(t, f, f.input, "commission-snapshot-first")
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay order=%+v first=%+v err=%v", replay, first, err)
	}
	if !bytes.Equal(read("commission_rule_snapshot", replay.ID), firstSnapshot) {
		t.Fatal("same-key replay did not preserve the original commission snapshot")
	}
	replayEvidence, err := readPolicySnapshot(f.brand, replay.ID)
	if err != nil || !reflect.DeepEqual(replayEvidence, firstEvidence) {
		t.Fatalf("same-key replay reader evidence=%+v original=%+v err=%v", replayEvidence, firstEvidence, err)
	}
	if walletAfterReplay := walletBySource(t, f); !reflect.DeepEqual(walletAfterReplay, walletBeforeReplay) {
		t.Fatalf("same-key replay changed wallet: before=%+v after=%+v", walletBeforeReplay, walletAfterReplay)
	}

	if _, err = f.db.Exec(ctx, `UPDATE bet_orders SET commission_rule_snapshot='{}'::jsonb,version=version+1 WHERE id=$1`, first.ID); err == nil {
		t.Fatal("commission rule snapshot rewrite was accepted")
	}
	public, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(public, []byte("commission_rule_snapshot")) {
		t.Fatal("private commission rule snapshot exposed in public order DTO")
	}
	var financialEntries int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type NOT IN ('adjustment','bet')`).Scan(&financialEntries); err != nil || financialEntries != 0 {
		t.Fatalf("commission policy configuration posted financial entries: count=%d err=%v", financialEntries, err)
	}

	historyBeforeFault, err := financial.History(ctx, f.brand, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	faultTx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = faultTx.Exec(ctx, `ALTER TABLE bet_orders DISABLE TRIGGER capture_bet_commission_snapshot`); err != nil {
		_ = faultTx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = faultTx.Exec(ctx, `ALTER TABLE bet_orders DISABLE TRIGGER immutable_bet_order`); err != nil {
		_ = faultTx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = faultTx.Exec(ctx, `UPDATE bet_orders SET commission_rule_snapshot=jsonb_set(commission_rule_snapshot,'{financial_policy,revision_id}',to_jsonb($2::text),false) WHERE id=$1`, first.ID, ids.New()); err != nil {
		_ = faultTx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = (commission.Source{}).PolicySnapshotTx(ctx, faultTx, f.brand, first.ID); !errors.Is(err, commission.ErrPolicyEvidence) {
		_ = faultTx.Rollback(ctx)
		t.Fatalf("mismatched immutable revision reference read error=%v, want policy evidence", err)
	}
	if err = faultTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	historyAfterFault, err := financial.History(ctx, f.brand, 10, 0)
	if err != nil || !reflect.DeepEqual(historyAfterFault, historyBeforeFault) {
		t.Fatalf("rollback fault injection changed policy history: before=%+v after=%+v err=%v", historyBeforeFault, historyAfterFault, err)
	}
}

type commissionSnapshotShape struct {
	SchemaVersion int    `json:"schema_version"`
	BrandID       string `json:"brand_id"`
	MemberID      string `json:"member_id"`
	CapturedAt    string `json:"captured_at"`
	Financial     struct {
		RevisionID string          `json:"revision_id"`
		Version    string          `json:"version"`
		Config     json.RawMessage `json:"config"`
	} `json:"financial_policy"`
	Agency struct {
		RevisionID string          `json:"revision_id"`
		Version    string          `json:"version"`
		Config     json.RawMessage `json:"config"`
	} `json:"agency_policy"`
	AgentPath []struct {
		ID            string          `json:"id"`
		MemberID      string          `json:"member_id"`
		ParentID      *string         `json:"parent_id"`
		Depth         int             `json:"depth"`
		RevisionID    string          `json:"revision_id"`
		Version       string          `json:"version"`
		Config        json.RawMessage `json:"config"`
		EffectiveMode string          `json:"effective_mode"`
	} `json:"agent_path"`
}

func assertCommissionSnapshot(t *testing.T, raw []byte, brand, member string, placedAt time.Time, financialRevision, financialVersion string, enabled bool, node agency.Node, nodeVersion, ratio, status string) {
	t.Helper()
	var got commissionSnapshotShape
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode commission snapshot: %v; data=%s", err, raw)
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, got.CapturedAt)
	if err != nil || !capturedAt.Equal(placedAt) {
		t.Fatalf("commission snapshot captured_at=%q, want placed_at=%s (parse error %v)", got.CapturedAt, placedAt, err)
	}
	if got.SchemaVersion != 1 || got.BrandID != brand || got.MemberID != member || got.Financial.RevisionID != financialRevision || got.Financial.Version != financialVersion {
		t.Fatalf("commission snapshot identity/financial policy=%+v", got)
	}
	var financialConfig map[string]any
	if err = json.Unmarshal(got.Financial.Config, &financialConfig); err != nil || financialConfig["enabled"] != enabled || financialConfig["payout_mode"] != commission.PayoutManual {
		t.Fatalf("financial config=%s err=%v", got.Financial.Config, err)
	}
	if enabled {
		calendar, ok := financialConfig["calendar"].(map[string]any)
		if !ok || calendar["timezone"] != "UTC" || calendar["cycle"] != "weekly" || calendar["boundary_time"] != "00:00:00" || calendar["weekday"] != float64(1) {
			t.Fatalf("enabled weekly UTC calendar=%v", financialConfig["calendar"])
		}
	} else if financialConfig["calendar"] != nil {
		t.Fatalf("disabled financial config calendar=%v, want explicit null", financialConfig["calendar"])
	}
	if len(got.AgentPath) != 1 {
		t.Fatalf("agent path length=%d, want one root agent", len(got.AgentPath))
	}
	pathNode := got.AgentPath[0]
	if pathNode.ID != node.ID || pathNode.MemberID != node.MemberID || pathNode.ParentID != nil || pathNode.Depth != 1 || pathNode.RevisionID == "" || pathNode.Version != nodeVersion || pathNode.EffectiveMode != "loss" {
		t.Fatalf("agent path entry=%+v", pathNode)
	}
	var nodeConfig map[string]any
	if err = json.Unmarshal(pathNode.Config, &nodeConfig); err != nil || nodeConfig["ratio"] != ratio || nodeConfig["status"] != status {
		t.Fatalf("agent path config=%s err=%v, want ratio %q and status %q", pathNode.Config, err, ratio, status)
	}
	var agencyConfig map[string]any
	if err = json.Unmarshal(got.Agency.Config, &agencyConfig); err != nil || agencyConfig["enabled"] != true {
		t.Fatalf("agency policy config=%s err=%v", got.Agency.Config, err)
	}
	if got.Agency.RevisionID == "" || got.Agency.Version != "2" {
		t.Fatalf("agency policy revision/version=%+v", got.Agency)
	}
}

func assertAttributionCommissionNull(t *testing.T, raw []byte) {
	t.Helper()
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if commissionValue, ok := snapshot["commission_policy"]; !ok || !bytes.Equal(bytes.TrimSpace(commissionValue), []byte("null")) {
		t.Fatalf("legacy attribution commission_policy changed: %s", raw)
	}
}

func updateCommissionSnapshotPolicy(ctx context.Context, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, service commission.Service, brand string, actor access.Account, input commission.PolicyInput) (commission.Policy, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return commission.Policy{}, err
	}
	defer tx.Rollback(ctx)
	updated, err := service.Update(ctx, tx, brand, actor, input, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
	if err != nil {
		return updated, err
	}
	if err = tx.Commit(ctx); err != nil {
		return updated, err
	}
	return updated, nil
}
