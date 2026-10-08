//go:build browserfixture

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	correctionFixtureGameCode = "commission_correction_fixture"
	correctionFixtureUser     = "commission_correction_user"
	correctionFreezeReason    = "owned browser correction fixture commission hold"
)

type correctionPrepareOutput struct {
	BrandID           string `json:"brand_id"`
	PlanID            string `json:"plan_id"`
	CycleID           string `json:"cycle_id"`
	OriginalPaymentID string `json:"original_payment_id"`
	BeneficiaryID     string `json:"beneficiary_id"`
}

type correctionFixtureIDs struct {
	CycleID, PeriodID, PaymentID string
	PlanID, ExecutionID          string
}

type correctionFixtureVerifyOutput struct {
	AdminID                   string  `json:"admin_id"`
	BrandID                   string  `json:"brand_id"`
	CycleID                   string  `json:"cycle_id"`
	PeriodID                  string  `json:"period_id"`
	OriginalPaymentID         string  `json:"original_payment_id"`
	PlanID                    string  `json:"plan_id"`
	PlanState                 string  `json:"plan_state"`
	DrawCorrectionID          string  `json:"draw_correction_id"`
	DrawCorrectionState       string  `json:"draw_correction_state"`
	CurrentJobID              *string `json:"current_job_id"`
	DrawTargetsPending        int64   `json:"draw_targets_pending"`
	DrawTargetsReversed       int64   `json:"draw_targets_reversed"`
	DrawTargetsUnchanged      int64   `json:"draw_targets_unchanged"`
	DrawTargetsExcluded       int64   `json:"draw_targets_excluded"`
	DrawTargetsFailed         int64   `json:"draw_targets_failed"`
	CorrectionTargetCount     string  `json:"correction_target_count"`
	CorrectionAppliedCount    string  `json:"correction_applied_count"`
	PlanTargetID              *string `json:"plan_target_id"`
	ExecutionTargetID         *string `json:"execution_target_id"`
	ExecutionLedgerEntryID    *string `json:"execution_ledger_entry_id"`
	ExecutionAuditLogID       *string `json:"execution_audit_log_id"`
	CorrectionDeltaPoints     string  `json:"correction_delta_points"`
	ExecutionID               *string `json:"execution_id"`
	State                     *string `json:"state"`
	CycleHoldActive           bool    `json:"cycle_hold_active"`
	Version                   *int64  `json:"version"`
	CorrectionLedgerEntries   int64   `json:"correction_ledger_entries"`
	CommissionWalletAvailable string  `json:"commission_wallet_available"`
	CommissionWalletFrozen    string  `json:"commission_wallet_frozen"`
	EconomicFingerprint       string  `json:"economic_fingerprint"`
}

func prepareCorrections(ctx context.Context, db *pgxpool.Pool, adminID, password string) (correctionPrepareOutput, error) {
	var out correctionPrepareOutput
	out.BrandID = fixtureBrand
	if len(password) < 16 {
		return out, errors.New("owned synthetic correction user password required")
	}
	var occupied bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM global_users WHERE username=$1)
 OR EXISTS(SELECT 1 FROM games WHERE brand_id=$2 AND code=$3)`, correctionFixtureUser, fixtureBrand, correctionFixtureGameCode).Scan(&occupied); err != nil {
		return out, errors.New("correction case uniqueness check failed")
	}
	if occupied {
		return out, errors.New("correction fixture already prepared; refusing to create a replacement case")
	}
	admin, err := (adminsys.Store{DB: db}).Account(ctx, adminID)
	if err != nil {
		return out, errors.New("owned fixture operator authorization unavailable")
	}
	var ownerMemberID, rootCode string
	err = db.QueryRow(ctx, `SELECT bm.id::text,c.code
 FROM global_users u JOIN brand_members bm ON bm.global_user_id=u.id AND bm.brand_id=$1
 JOIN agent_nodes n ON n.brand_id=bm.brand_id AND n.member_id=bm.id
 JOIN join_codes c ON c.brand_id=n.brand_id AND c.agent_id=n.id AND c.kind='agent' AND c.status='active'
	 WHERE u.username=$2 ORDER BY c.created_at,c.id LIMIT 1`, fixtureBrand, fixtureOwner).Scan(&ownerMemberID, &rootCode)
	if err != nil {
		return out, errors.New("owned root agent referral code unavailable")
	}
	identityStore, err := identity.New(db)
	if err != nil {
		return out, errors.New("correction fixture identity service unavailable")
	}
	registered, err := register(ctx, db, identityStore, correctionFixtureUser, password, rootCode)
	if err != nil {
		return out, errors.New("correction fixture beneficiary registration failed")
	}
	session, err := identityStore.Authenticate(ctx, fixtureBrand, registered.AccessToken)
	if err != nil {
		return out, errors.New("correction fixture beneficiary authentication failed")
	}
	var seed points.Balance
	seed[2][0] = 20
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: fixtureBrand, MemberID: session.Member.ID, EntryType: "adjustment", ReferenceType: "test_fixture", ReferenceID: session.Member.ID,
			OperationKey: "commission-correction-fixture-fund:" + session.Member.ID, Reason: "owned synthetic correction fixture funding", ActorType: "system", RequestID: ids.New(), Delta: seed,
			Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 20}}})
		return e
	}); err != nil {
		return out, errors.New("correction fixture audited wallet funding failed")
	}
	reviewer, err := reviewerAccount(ctx, db)
	if err != nil {
		return out, err
	}
	meta := func(actor string) points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: ids.New()}
	}
	game, play, version, err := createRuleWorkflowNamed(ctx, db, admin, reviewer, meta, correctionFixtureGameCode)
	if err != nil {
		return out, err
	}
	betService, finance := betting.Service{DB: db}, commission.Service{DB: db}
	phase, err := runPeriod(ctx, db, admin, game, play, version, session, betService, finance, 1, "correction-manual")
	if err != nil {
		return out, fmt.Errorf("correction fixture period: %w", err)
	}
	if err = setOriginalPaymentGate(ctx, db, admin, meta(adminID)); err != nil {
		return out, err
	}
	if _, err = finance.ProcessPayments(ctx, 100); err != nil {
		return out, errors.New("original commission payout worker failed")
	}
	paymentID, err := paymentForCycle(ctx, db, phase.cycleID)
	if err != nil {
		return out, errors.New("original commission payment was not created")
	}
	var payment commission.Payment
	err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		payment, e = finance.PaymentTx(ctx, tx, fixtureBrand, paymentID)
		return e
	})
	if err != nil || payment.State != "awaiting_approval" || payment.PayoutMode != commission.PayoutManual || payment.TotalPoints != "1" {
		return out, errors.New("original manual commission payment did not await review for one point")
	}
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		payment, e = finance.ApprovePaymentTx(ctx, tx, fixtureBrand, payment.ID, admin,
			commission.RetryCycleInput{Version: payment.Version, Reason: "explicitly approve original synthetic commission payout"}, meta(adminID))
		return e
	}); err != nil {
		return out, errors.New("original commission payment approval failed")
	}
	if _, err = finance.ProcessPayments(ctx, 100); err != nil {
		return out, errors.New("approved original commission payment worker failed")
	}
	err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		payment, e = finance.PaymentTx(ctx, tx, fixtureBrand, payment.ID)
		return e
	})
	if err != nil || payment.State != "paid" || payment.PaidPoints != "1" {
		return out, errors.New("original commission payment was not actually credited")
	}
	if err = createActualDrawCorrection(ctx, db, admin, phase.periodID, meta(adminID)); err != nil {
		return out, err
	}
	if err = completeActualDrawCorrection(ctx, db, phase.periodID, finance, betService, admin, meta(adminID)); err != nil {
		return out, err
	}
	if _, err = finance.ProcessDiscovery(ctx, 100); err != nil {
		return out, errors.New("corrected commission discovery failed")
	}
	if err = advanceCycles(ctx, finance); err != nil {
		return out, errors.New("corrected commission cycle processing failed")
	}
	if _, err = finance.ProcessPayments(ctx, 100); err != nil {
		return out, errors.New("original payment invalidation worker failed")
	}
	if _, err = finance.ProcessCorrectionPlans(ctx, 100); err != nil {
		return out, errors.New("commission correction plan worker failed")
	}
	plan, err := correctionPlanForCycle(ctx, db, phase.cycleID)
	if err != nil || plan.State != commission.CorrectionPlanReady || plan.PaymentID != payment.ID || plan.PayoutMode != commission.PayoutManual || plan.BeforePoints != "1" || plan.CalculatedPoints != "0" || plan.DebitPoints == nil || *plan.DebitPoints != "1" || plan.NetPoints == nil || *plan.NetPoints != "-1" {
		return out, errors.New("actual manual correction difference did not become a ready one-point debit plan")
	}
	if correctionPolicyEnabled(ctx, db) {
		return out, errors.New("independent correction execution gate was unexpectedly enabled")
	}
	out.PlanID, out.CycleID, out.OriginalPaymentID, out.BeneficiaryID = plan.ID, phase.cycleID, payment.ID, ownerMemberID
	return out, nil
}

func reviewerAccount(ctx context.Context, db *pgxpool.Pool) (access.Account, error) {
	var id string
	if err := db.QueryRow(ctx, `SELECT id::text FROM admin_accounts WHERE username='commission_rule_reviewer' AND status='active'`).Scan(&id); err != nil {
		return access.Account{}, errors.New("owned fixture rule reviewer unavailable")
	}
	a, err := (adminsys.Store{DB: db}).Account(ctx, id)
	if err != nil {
		return access.Account{}, errors.New("owned fixture rule reviewer authorization unavailable")
	}
	return a, nil
}

func setOriginalPaymentGate(ctx context.Context, db *pgxpool.Pool, admin access.Account, meta points.Metadata) error {
	service := commission.Service{DB: db}
	return inTx(ctx, db, func(tx pgx.Tx) error {
		policy, err := service.PaymentPolicyTx(ctx, tx, fixtureBrand)
		if err != nil {
			return err
		}
		if policy.Enabled {
			return nil
		}
		_, err = service.UpdatePaymentPolicyTx(ctx, tx, fixtureBrand, admin,
			commission.PaymentPolicyInput{Version: policy.Version, Enabled: true, Reason: "explicitly enable original synthetic commission payout for correction case"}, meta)
		return err
	})
}

func paymentForCycle(ctx context.Context, db *pgxpool.Pool, cycleID string) (string, error) {
	service := commission.Service{DB: db}
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	page, err := service.PaymentsTx(ctx, tx, fixtureBrand, 100, 0)
	if err != nil {
		return "", err
	}
	for _, payment := range page.Items {
		if payment.CycleID == cycleID {
			return payment.ID, nil
		}
	}
	return "", commission.ErrNotFound
}

func createActualDrawCorrection(ctx context.Context, db *pgxpool.Pool, admin access.Account, periodID string, meta points.Metadata) error {
	service := betting.Service{DB: db}
	context, err := service.CorrectionContext(ctx, fixtureBrand, periodID)
	if err != nil || !context.CanCorrect || context.DrawResultID == nil || context.PolicyVersion < 1 {
		return errors.New("actual corrected draw preconditions unavailable")
	}
	policyVersion := context.PolicyVersion
	return inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := service.CreateCorrection(ctx, tx, fixtureBrand, admin, *context.DrawResultID,
			betting.CorrectionInput{Version: context.PeriodVersion, PolicyVersion: &policyVersion,
				Result: rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{0, 0, 0}}, Reason: "replace actual synthetic losing draw with matching result"}, meta)
		return e
	})
}

func completeActualDrawCorrection(ctx context.Context, db *pgxpool.Pool, periodID string, finance commission.Service, service betting.Service, admin access.Account, meta points.Metadata) error {
	var correction betting.Correction
	for step := 0; step < 24; step++ {
		page, err := service.Corrections(ctx, fixtureBrand, periodID, 10, 0)
		if err != nil || len(page.Items) != 1 {
			return errors.New("actual draw correction record unavailable")
		}
		correction = page.Items[0]
		if correction.State == "completed" {
			return nil
		}
		if correction.State == "reversing" || correction.State == "resettling" {
			if _, err = service.ProcessCorrections(ctx, 20); err != nil {
				return errors.New("actual draw correction worker failed")
			}
		}
		if correction.NewJobID != nil {
			job, jobErr := service.SettlementJob(ctx, fixtureBrand, *correction.NewJobID)
			if jobErr == nil && job.State == "awaiting_approval" {
				err = inTx(ctx, db, func(tx pgx.Tx) error {
					var e error
					job, e = service.ActOnSettlement(ctx, tx, fixtureBrand, admin, job.ID, "approve",
						betting.SettlementActionInput{Version: job.Version, Reason: "approve actual corrected synthetic period settlement"}, meta)
					return e
				})
				if err != nil {
					return errors.New("corrected period settlement approval failed")
				}
			}
			if jobErr == nil && (job.State == "processing" || job.State == "paying" || job.State == "awaiting_approval") {
				if _, err = service.ProcessSettlements(ctx, 100); err != nil {
					return errors.New("corrected period settlement worker failed")
				}
			}
		}
		if _, err := finance.ProcessDiscovery(ctx, 100); err != nil {
			return errors.New("corrected commission discovery worker failed")
		}
	}
	return errors.New("actual draw correction did not complete within bounded steps")
}

func correctionCaseIDs(ctx context.Context, db *pgxpool.Pool) (correctionFixtureIDs, error) {
	var out correctionFixtureIDs
	err := db.QueryRow(ctx, `SELECT c.id::text,p.id::text
 FROM games g JOIN periods p ON p.brand_id=g.brand_id AND p.game_id=g.id
 JOIN bet_orders o ON o.brand_id=p.brand_id AND o.period_id=p.id
 JOIN commission_cycles c ON c.brand_id=o.brand_id AND c.anchor_order_id=o.id
 WHERE g.brand_id=$1 AND g.code=$2 ORDER BY p.created_at,p.id LIMIT 1`, fixtureBrand, correctionFixtureGameCode).Scan(&out.CycleID, &out.PeriodID)
	if err != nil {
		return out, err
	}
	out.PaymentID, err = paymentForCycle(ctx, db, out.CycleID)
	if err != nil {
		return out, err
	}
	plan, err := correctionPlanForCycle(ctx, db, out.CycleID)
	if err != nil {
		return out, err
	}
	out.PlanID = plan.ID
	return out, nil
}

func correctionPlanForCycle(ctx context.Context, db *pgxpool.Pool, cycleID string) (commission.CorrectionPlan, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return commission.CorrectionPlan{}, err
	}
	defer tx.Rollback(ctx)
	page, err := (commission.Service{DB: db}).CorrectionPlansTx(ctx, tx, fixtureBrand, 100, 0)
	if err != nil {
		return commission.CorrectionPlan{}, err
	}
	for _, plan := range page.Items {
		if plan.CycleID == cycleID && plan.State != commission.CorrectionPlanStale {
			return plan, nil
		}
	}
	return commission.CorrectionPlan{}, commission.ErrNotFound
}

func correctionExecutionForPlan(ctx context.Context, db *pgxpool.Pool, planID string) (*commission.CorrectionExecution, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	page, err := (commission.Service{DB: db}).CorrectionExecutionsTx(ctx, tx, fixtureBrand, 100, 0)
	if err != nil {
		return nil, err
	}
	for i := range page.Items {
		if page.Items[i].PlanID == planID {
			return &page.Items[i], nil
		}
	}
	return nil, nil
}

func correctionPolicyEnabled(ctx context.Context, db *pgxpool.Pool) bool {
	tx, err := db.Begin(ctx)
	if err != nil {
		return true
	}
	defer tx.Rollback(ctx)
	policy, err := (commission.Service{DB: db}).CorrectionExecutionPolicyTx(ctx, tx, fixtureBrand)
	return err != nil || policy.Enabled
}

func correctionLedgerEntry(ctx context.Context, db *pgxpool.Pool, memberID, entryID string) (points.Entry, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return points.Entry{}, err
	}
	defer tx.Rollback(ctx)
	return (points.Store{DB: db}).Entry(ctx, tx, fixtureBrand, memberID, entryID)
}

func correctionNegativePlanTarget(ctx context.Context, db *pgxpool.Pool, planID string) (commission.CorrectionPlanTarget, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return commission.CorrectionPlanTarget{}, err
	}
	defer tx.Rollback(ctx)
	page, err := (commission.Service{DB: db}).CorrectionPlanTargetsTx(ctx, tx, fixtureBrand, planID, 100, 0)
	if err != nil {
		return commission.CorrectionPlanTarget{}, err
	}
	if len(page.Items) != 1 || page.Items[0].DeltaPoints >= 0 {
		return commission.CorrectionPlanTarget{}, errors.New("correction plan must have exactly one negative beneficiary target")
	}
	return page.Items[0], nil
}

func freezeCorrections(ctx context.Context, db *pgxpool.Pool, adminID string) (map[string]any, error) {
	if err := requireFreezeAuthority(ctx, db, adminID); err != nil {
		return nil, err
	}
	idsForCase, err := correctionCaseIDs(ctx, db)
	if err != nil {
		return nil, errors.New("correction fixture is not prepared")
	}
	execution, err := correctionExecutionForPlan(ctx, db, idsForCase.PlanID)
	if err != nil || execution == nil {
		return nil, errors.New("correction execution has not been created")
	}
	target, err := correctionNegativePlanTarget(ctx, db, execution.PlanID)
	if err != nil {
		return nil, errors.New("correction plan does not have one negative beneficiary target")
	}
	key := "commission-correction-fixture-freeze:" + execution.ID
	var existing string
	err = db.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND operation_key=$2`, fixtureBrand, key).Scan(&existing)
	if err == nil {
		entry, readErr := correctionLedgerEntry(ctx, db, target.MemberID, existing)
		if readErr != nil {
			return nil, errors.New("existing correction freeze ledger read failed")
		}
		return map[string]any{"ledger_entry_id": entry.ID, "frozen_points": strconv.FormatInt(int64(entry.Delta[3][1]), 10)}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("correction freeze idempotency lookup failed")
	}
	wallet, err := (points.Store{DB: db}).Read(ctx, fixtureBrand, target.MemberID)
	if err != nil || wallet.BySource[3][0] <= 0 {
		return nil, errors.New("beneficiary has no available commission points to hold")
	}
	amount := wallet.BySource[3][0]
	allocation := []points.Allocation{{Source: "commission", State: "available", Points: amount}}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		return nil, errors.New("commission freeze allocation invalid")
	}
	var entry points.Entry
	err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		entry, e = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: fixtureBrand, MemberID: target.MemberID, EntryType: "freeze", ReferenceType: "commission_correction_fixture", ReferenceID: execution.ID,
			OperationKey: key, Reason: correctionFreezeReason, ActorType: "admin", ActorID: adminID, RequestID: ids.New(), Delta: delta, Allocation: allocation})
		return e
	})
	if err != nil {
		return nil, errors.New("normal commission-source freeze ledger posting failed")
	}
	return map[string]any{"ledger_entry_id": entry.ID, "frozen_points": strconv.FormatInt(int64(amount), 10)}, nil
}

func unfreezeCorrections(ctx context.Context, db *pgxpool.Pool, adminID string) (map[string]any, error) {
	if err := requireFreezeAuthority(ctx, db, adminID); err != nil {
		return nil, err
	}
	idsForCase, err := correctionCaseIDs(ctx, db)
	if err != nil {
		return nil, errors.New("correction fixture is not prepared")
	}
	execution, err := correctionExecutionForPlan(ctx, db, idsForCase.PlanID)
	if err != nil || execution == nil {
		return nil, errors.New("correction execution has not been created")
	}
	target, err := correctionNegativePlanTarget(ctx, db, execution.PlanID)
	if err != nil {
		return nil, errors.New("correction plan beneficiary unavailable")
	}
	memberID := target.MemberID
	freezeKey := "commission-correction-fixture-freeze:" + execution.ID
	var freezeID string
	if err = db.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND operation_key=$2 AND entry_type='freeze'`, fixtureBrand, freezeKey).Scan(&freezeID); err != nil {
		return nil, errors.New("original correction freeze ledger entry not found")
	}
	reversalKey := "commission-correction-fixture-unfreeze:" + execution.ID
	var priorID string
	err = db.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND operation_key=$2`, fixtureBrand, reversalKey).Scan(&priorID)
	if err == nil {
		return map[string]any{"ledger_entry_id": priorID, "reversed_entry_id": freezeID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("commission unfreeze idempotency lookup failed")
	}
	var reversed points.Entry
	err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		reversed, e = (points.Store{DB: db}).Reverse(ctx, tx, fixtureBrand, memberID, freezeID, reversalKey, "restore original commission allocation after held execution review", points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
		return e
	})
	if err != nil {
		return nil, errors.New("normal ledger reversal of original freeze failed")
	}
	return map[string]any{"ledger_entry_id": reversed.ID, "reversed_entry_id": freezeID}, nil
}

func verifyCorrections(ctx context.Context, db *pgxpool.Pool, adminID string) (correctionFixtureVerifyOutput, error) {
	var out correctionFixtureVerifyOutput
	out.AdminID, out.BrandID = adminID, fixtureBrand
	idsForCase, err := correctionCaseIDs(ctx, db)
	if err != nil {
		return out, err
	}
	out.CycleID, out.PeriodID, out.OriginalPaymentID, out.PlanID = idsForCase.CycleID, idsForCase.PeriodID, idsForCase.PaymentID, idsForCase.PlanID
	drawPage, err := (betting.Service{DB: db}).Corrections(ctx, fixtureBrand, idsForCase.PeriodID, 10, 0)
	if err != nil || len(drawPage.Items) != 1 {
		return out, errors.New("actual draw correction snapshot unavailable")
	}
	drawCorrection := drawPage.Items[0]
	out.DrawCorrectionID, out.DrawCorrectionState = drawCorrection.ID, drawCorrection.State
	out.DrawTargetsPending, out.DrawTargetsReversed = drawCorrection.PendingCount, drawCorrection.ReversedCount
	out.DrawTargetsUnchanged, out.DrawTargetsExcluded, out.DrawTargetsFailed = drawCorrection.UnchangedCount, drawCorrection.ExcludedCount, drawCorrection.FailedCount
	periodContext, err := (betting.Service{DB: db}).CorrectionContext(ctx, fixtureBrand, idsForCase.PeriodID)
	if err != nil {
		return out, err
	}
	out.CurrentJobID = periodContext.CurrentJobID
	planTx, err := db.Begin(ctx)
	if err != nil {
		return out, err
	}
	planValue, err := (commission.Service{DB: db}).CorrectionPlanTx(ctx, planTx, fixtureBrand, idsForCase.PlanID)
	_ = planTx.Rollback(ctx)
	if err != nil {
		return out, err
	}
	out.PlanState, out.CorrectionTargetCount = planValue.State, planValue.TargetCount
	execution, err := correctionExecutionForPlan(ctx, db, idsForCase.PlanID)
	if err != nil {
		return out, err
	}
	planTargetTx, beginErr := db.Begin(ctx)
	if beginErr != nil {
		return out, beginErr
	}
	planTargets, readErr := (commission.Service{DB: db}).CorrectionPlanTargetsTx(ctx, planTargetTx, fixtureBrand, planValue.ID, 100, 0)
	_ = planTargetTx.Rollback(ctx)
	if readErr != nil || len(planTargets.Items) != 1 {
		return out, errors.New("correction plan target verification failed")
	}
	planTarget := planTargets.Items[0]
	if planTarget.BrandID != fixtureBrand || planTarget.PlanID != planValue.ID || planTarget.DeltaPoints >= 0 {
		return out, errors.New("correction plan beneficiary target mismatch")
	}
	beneficiary := planTarget.MemberID
	out.PlanTargetID = &planTarget.ID
	out.CorrectionDeltaPoints = strconv.FormatInt(int64(planTarget.DeltaPoints), 10)
	if execution != nil {
		out.ExecutionID, out.State, out.Version = &execution.ID, &execution.State, &execution.Version
		out.CycleHoldActive = execution.CycleHoldActive
		out.CorrectionAppliedCount = execution.AppliedCount
		var appliedTargets []commission.CorrectionExecutionTarget
		if execution.AppliedCount == "1" {
			tx, targetBeginErr := db.Begin(ctx)
			if targetBeginErr != nil {
				return out, targetBeginErr
			}
			targets, targetReadErr := (commission.Service{DB: db}).CorrectionExecutionTargetsTx(ctx, tx, fixtureBrand, execution.ID, 100, 0)
			_ = tx.Rollback(ctx)
			if targetReadErr != nil {
				return out, errors.New("applied correction execution target read failed")
			}
			appliedTargets = targets.Items
		}
		target, targetErr := verifiedCorrectionExecutionTarget(execution.AppliedCount, execution.ID, planTarget, appliedTargets)
		if targetErr != nil {
			return out, targetErr
		}
		if target != nil {
			out.ExecutionTargetID, out.ExecutionLedgerEntryID, out.ExecutionAuditLogID = &target.ID, target.LedgerEntryID, target.AuditLogID
		}
	} else {
		out.CorrectionAppliedCount = "0"
	}
	wallet, err := (points.Store{DB: db}).Read(ctx, fixtureBrand, beneficiary)
	if err != nil {
		return out, err
	}
	out.CommissionWalletAvailable = strconv.FormatInt(int64(wallet.BySource[3][0]), 10)
	out.CommissionWalletFrozen = strconv.FormatInt(int64(wallet.BySource[3][1]), 10)
	var executionID *string
	if execution != nil {
		executionID = &execution.ID
	}
	out.CorrectionLedgerEntries, err = correctionLedgerEntryCount(ctx, db, executionID)
	if err != nil {
		return out, err
	}
	out.EconomicFingerprint, err = economicFingerprint(ctx, db)
	if err != nil {
		return out, err
	}
	return out, nil
}

func verifiedCorrectionExecutionTarget(appliedCount, executionID string, planTarget commission.CorrectionPlanTarget, targets []commission.CorrectionExecutionTarget) (*commission.CorrectionExecutionTarget, error) {
	switch appliedCount {
	case "0":
		if len(targets) != 0 {
			return nil, errors.New("unapplied correction unexpectedly has execution targets")
		}
		// No execution target exists until the core applies the correction.
		return nil, nil
	case "1":
		if len(targets) != 1 {
			return nil, errors.New("applied correction execution target count mismatch")
		}
		target := &targets[0]
		if target.BrandID != fixtureBrand || target.ExecutionID != executionID || target.PlanTargetID != planTarget.ID || target.MemberID != planTarget.MemberID || target.DeltaPoints != planTarget.DeltaPoints || target.State != commission.CorrectionExecutionTargetApplied || target.LedgerEntryID == nil || *target.LedgerEntryID == "" || target.AuditLogID == nil || *target.AuditLogID == "" {
			return nil, errors.New("applied correction execution target does not match its plan target")
		}
		return target, nil
	default:
		return nil, errors.New("correction execution applied count is outside fixture expectations")
	}
}

func correctionLedgerEntryCount(ctx context.Context, db *pgxpool.Pool, executionID *string) (int64, error) {
	if executionID == nil {
		// An approved plan has no execution target ledger entries yet. Avoid
		// binding an empty string to PostgreSQL's UUID parameter type.
		return 0, nil
	}
	var count int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries l WHERE l.brand_id=$1 AND l.entry_type='commission_correction' AND l.reference_type='commission_correction_target' AND EXISTS(SELECT 1 FROM commission_correction_execution_targets t WHERE t.brand_id=l.brand_id AND t.id=l.reference_id AND t.execution_id=$2::uuid)`, fixtureBrand, *executionID).Scan(&count)
	return count, err
}

func requireFreezeAuthority(ctx context.Context, db *pgxpool.Pool, adminID string) error {
	a, err := (adminsys.Store{DB: db}).Account(ctx, adminID)
	if err != nil || a.SuperAdmin || a.Type != access.AccountAdmin || !access.Authorize(a, "wallet", "freeze", access.ScopeBrand, fixtureBrand) {
		return errors.New("ordinary brand-scoped wallet freeze permission required")
	}
	return nil
}
