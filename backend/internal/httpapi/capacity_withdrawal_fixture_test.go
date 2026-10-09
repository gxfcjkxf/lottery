//go:build capacity && !windows

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/capacity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

// prepareCapacityWithdrawals enables one real, brand-scoped operator per
// fixture brand. The grants are persisted because OrderService.Advance reloads
// each operator's authorization from the database inside its transaction.
func prepareCapacityWithdrawals(t *testing.T, f capacityFixture) map[string]access.Account {
	t.Helper()
	ctx := context.Background()
	actors := make(map[string]access.Account)
	admins := adminsys.Store{DB: f.DB}
	policies := withdrawal.Service{DB: f.DB}
	permissionKeys := []string{
		"withdrawal_policy.write.brand",
		"withdrawal.approve.brand",
		"withdrawal.mark_paid.brand",
	}

	for _, user := range f.Users {
		if _, exists := actors[user.Brand]; exists {
			continue
		}
		var brandID, adminID string
		if err := f.DB.QueryRow(ctx, `SELECT b.id::text,g.created_by::text
			FROM brands b JOIN games g ON g.brand_id=b.id
			WHERE b.code=$1 AND g.code='capacity_game' ORDER BY g.id LIMIT 1`, user.Brand).Scan(&brandID, &adminID); err != nil {
			t.Fatalf("find capacity game operator for %s: %v", user.Brand, err)
		}
		roleID := ids.New()
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			var permissionCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE key=ANY($1::text[])`, permissionKeys).Scan(&permissionCount); err != nil {
				return err
			}
			if permissionCount != len(permissionKeys) {
				return fmt.Errorf("fixture database has %d of %d required withdrawal permissions", permissionCount, len(permissionKeys))
			}
			if _, err := tx.Exec(ctx, `INSERT INTO roles(id,code,name,brand_id) VALUES($1,$2,$3,$4)`, roleID, "capacity_withdrawal_"+strings.ReplaceAll(roleID, "-", ""), "Capacity withdrawal operator", brandID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT $1,key FROM permissions WHERE key=ANY($2::text[])`, roleID, permissionKeys); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, adminID, brandID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roleID)
			return err
		})

		actor, err := admins.Account(ctx, adminID)
		if err != nil {
			t.Fatalf("load persisted capacity withdrawal operator for %s: %v", user.Brand, err)
		}
		if !access.Authorize(actor, "withdrawal_policy", "write", access.ScopeBrand, brandID) ||
			!access.Authorize(actor, "withdrawal", "approve", access.ScopeBrand, brandID) ||
			!access.Authorize(actor, "withdrawal", "mark_paid", access.ScopeBrand, brandID) {
			t.Fatalf("persisted capacity operator lacks required grants for %s", user.Brand)
		}

		policy, err := policies.BrandPolicy(ctx, brandID)
		if err != nil {
			t.Fatalf("load withdrawal policy for %s: %v", user.Brand, err)
		}
		config := withdrawal.BrandConfig{
			Enabled:          true,
			MinPoints:        1,
			AllowedSources:   []string{"recharge"},
			ReviewMode:       "manual",
			TurnoverMultiple: "0.000001",
		}
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			_, err := policies.UpdateBrand(ctx, tx, brandID, actor, withdrawal.BrandInput{
				Version: policy.Version,
				Config:  config,
				Reason:  "isolated capacity withdrawal fixture",
			}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
			return err
		})
		actors[user.Brand] = actor
	}
	return actors
}

type capacityWithdrawalEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   struct {
		Code string `json:"code"`
	} `json:"error"`
}

func capacityWithdrawalRequest(ctx context.Context, f capacityFixture, user capacityUser, method, path string, body []byte, idempotencyKey, actorContext string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, f.Server.URL+"/api/v1/b/"+user.Brand+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+user.Token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if actorContext != "" {
		req.Header.Set("X-Withdrawal-Actor-Context", actorContext)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, err
}

// processCapacityWithdrawals submits exactly one real request for each of the
// 500 shared fixture members, then advances each order through the persisted
// operator ACL in independent transactions.
func processCapacityWithdrawals(ctx context.Context, f capacityFixture, actors map[string]access.Account) (capacity.Report, error) {
	if len(f.Users) != 500 {
		return capacity.Report{}, fmt.Errorf("withdrawal capacity requires exactly 500 fixture users, got %d", len(f.Users))
	}
	brandIDs := make(map[string]string, len(actors))
	for code, actor := range actors {
		if len(actor.BrandIDs) != 1 {
			return capacity.Report{}, fmt.Errorf("capacity operator for brand %s has %d persisted scopes", code, len(actor.BrandIDs))
		}
		brandIDs[code] = actor.BrandIDs[0]
	}
	service := withdrawal.OrderService{DB: f.DB, Points: points.Store{DB: f.DB}, Eligibility: withdrawal.TurnoverChecker{}}
	return capacity.Run(ctx, capacity.Plan{Rate: 100, Duration: 5 * time.Second, MaxInFlight: 20, RequestTimeout: 10 * time.Second}, func(requestCtx context.Context, index int) capacity.Observation {
		user := f.Users[index]
		actor, ok := actors[user.Brand]
		if !ok || actor.ID == "" {
			return capacity.Observation{Err: "missing_persisted_admin_actor"}
		}
		status, raw, err := capacityWithdrawalRequest(requestCtx, f, user, http.MethodGet, "/withdrawal-availability", nil, "", "")
		if err != nil {
			return capacity.Observation{Status: status, Err: "availability_request_failed"}
		}
		var availabilityEnvelope capacityWithdrawalEnvelope
		var availability withdrawal.AvailabilityView
		if json.Unmarshal(raw, &availabilityEnvelope) != nil || !availabilityEnvelope.Success || json.Unmarshal(availabilityEnvelope.Data, &availability) != nil {
			return capacity.Observation{Status: status, Err: "invalid_availability_receipt"}
		}
		if status != http.StatusOK || !availability.PolicyEnabled || !availability.EligibilityConfigured || !availability.CanApply || availability.RealPayments || availability.MemberID != user.Member || availability.ActorContext == "" {
			return capacity.Observation{Status: status, Err: "unexpected_availability"}
		}
		body, err := json.Marshal(withdrawal.CreateRequest{Points: 50, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 50}}})
		if err != nil {
			return capacity.Observation{Err: "withdrawal_request_encode_failed"}
		}
		status, raw, err = capacityWithdrawalRequest(requestCtx, f, user, http.MethodPost, "/withdrawals", body, "capacity-withdraw-"+user.Member, availability.ActorContext)
		if err != nil {
			return capacity.Observation{Status: status, Err: "withdrawal_request_failed"}
		}
		var createdEnvelope capacityWithdrawalEnvelope
		var created withdrawal.OrderView
		if json.Unmarshal(raw, &createdEnvelope) != nil || !createdEnvelope.Success || json.Unmarshal(createdEnvelope.Data, &created) != nil {
			return capacity.Observation{Status: status, Err: "invalid_withdrawal_receipt"}
		}
		if status != http.StatusCreated || !uuidPattern.MatchString(created.ID) || created.MemberID != user.Member || created.State != "reviewing" || created.Version != 1 || created.Points != 50 {
			return capacity.Observation{Status: status, Err: "unexpected_withdrawal_receipt"}
		}

		for _, step := range []struct{ action, expectedState string }{{"approve", "processing"}, {"mark_paid", "paid"}} {
			tx, beginErr := f.DB.Begin(requestCtx)
			if beginErr != nil {
				return capacity.Observation{Err: "withdrawal_transition_begin_failed"}
			}
			next, advanceErr := service.Advance(requestCtx, tx, brandIDs[user.Brand], created.ID, step.action, actor, withdrawal.ActionInput{
				Version: created.Version, ClientKey: "capacity-withdraw-" + step.action + "-" + user.Member, Reason: "capacity withdrawal workflow",
			}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
			if advanceErr != nil {
				_ = tx.Rollback(requestCtx)
				return capacity.Observation{Err: "withdrawal_transition_failed"}
			}
			if next.State != step.expectedState {
				_ = tx.Rollback(requestCtx)
				return capacity.Observation{Status: status, Err: "unexpected_withdrawal_transition"}
			}
			if commitErr := tx.Commit(requestCtx); commitErr != nil {
				return capacity.Observation{Err: "withdrawal_transition_commit_failed"}
			}
			created.Version = next.Version
			created.State = next.State
		}
		if created.State != "paid" {
			return capacity.Observation{Status: status, Err: "withdrawal_not_paid"}
		}
		return capacity.Observation{Status: status, OrderID: created.ID}
	})
}

func TestCapacityWithdrawalFixtureSmoke(t *testing.T) {
	f := newCapacityFixture(t, 4, 2, 20)
	ctx := context.Background()
	var beforeOrders, beforeLedger int64
	if err := f.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM withdrawal_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type IN ('withdrawal_reserve','withdrawal_paid'))`).Scan(&beforeOrders, &beforeLedger); err != nil {
		t.Fatal(err)
	}
	actors := prepareCapacityWithdrawals(t, f)
	if len(actors) != 2 {
		t.Fatalf("prepared %d brand operators; want 2", len(actors))
	}
	for _, user := range f.Users {
		actor := actors[user.Brand]
		var brandID string
		if err := f.DB.QueryRow(ctx, `SELECT id::text FROM brands WHERE code=$1`, user.Brand).Scan(&brandID); err != nil {
			t.Fatal(err)
		}
		persisted, err := (adminsys.Store{DB: f.DB}).Account(ctx, actor.ID)
		if err != nil || !access.Authorize(persisted, "withdrawal", "approve", access.ScopeBrand, brandID) || !access.Authorize(persisted, "withdrawal", "mark_paid", access.ScopeBrand, brandID) {
			t.Fatalf("persisted actor authorization missing for brand %s: %v", user.Brand, err)
		}
		policy, err := (withdrawal.Service{DB: f.DB}).BrandPolicy(ctx, brandID)
		if err != nil || !policy.Config.Enabled || policy.Config.MinPoints != 1 || len(policy.Config.AllowedSources) != 1 || policy.Config.AllowedSources[0] != "recharge" || policy.Config.ReviewMode != "manual" || policy.Config.TurnoverMultiple != "0.000001" {
			t.Fatalf("unexpected persisted withdrawal policy for brand %s: %+v (%v)", user.Brand, policy, err)
		}
	}
	var afterOrders, afterLedger int64
	if err := f.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM withdrawal_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type IN ('withdrawal_reserve','withdrawal_paid'))`).Scan(&afterOrders, &afterLedger); err != nil {
		t.Fatal(err)
	}
	if afterOrders != beforeOrders || afterLedger != beforeLedger {
		t.Fatalf("preparation changed withdrawal business facts: orders %d->%d, ledger %d->%d", beforeOrders, afterOrders, beforeLedger, afterLedger)
	}
}
