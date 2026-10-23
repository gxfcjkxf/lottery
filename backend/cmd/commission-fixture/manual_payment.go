//go:build browserfixture

package main

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Build an actual positive manual payout and stop before its approval.
func prepareManualPayment(ctx context.Context, db *pgxpool.Pool, adminID, password string) (map[string]string, error) {
	const username = "commission_manual_positive_user"
	const gameCode = "commission_manual_positive_fixture"
	var occupied bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM global_users WHERE username=$1) OR EXISTS(SELECT 1 FROM games WHERE brand_id=$2 AND code=$3)`, username, fixtureBrand, gameCode).Scan(&occupied); err != nil {
		return nil, err
	}
	if occupied {
		return nil, errors.New("positive manual fixture already exists; refusing to replace it")
	}
	admin, err := (adminsys.Store{DB: db}).Account(ctx, adminID)
	if err != nil {
		return nil, err
	}
	reviewer, err := reviewerAccount(ctx, db)
	if err != nil {
		return nil, err
	}
	var rootCode string
	if err = db.QueryRow(ctx, `SELECT c.code FROM join_codes c JOIN agent_nodes n ON n.id=c.agent_id AND n.brand_id=c.brand_id JOIN brand_members m ON m.id=n.member_id JOIN global_users u ON u.id=m.global_user_id WHERE c.brand_id=$1 AND u.username=$2 AND c.kind='agent' AND c.status='active' ORDER BY c.created_at,c.id LIMIT 1`, fixtureBrand, fixtureOwner).Scan(&rootCode); err != nil {
		return nil, err
	}
	store, err := identity.New(db)
	if err != nil {
		return nil, err
	}
	registered, err := register(ctx, db, store, username, password, rootCode)
	if err != nil {
		return nil, err
	}
	session, err := store.Authenticate(ctx, fixtureBrand, registered.AccessToken)
	if err != nil {
		return nil, err
	}
	var seed points.Balance
	seed[2][0] = 20
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		_, e := (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: fixtureBrand, MemberID: session.Member.ID, EntryType: "adjustment", ReferenceType: "test_fixture", ReferenceID: session.Member.ID, OperationKey: "manual-positive-fixture:" + session.Member.ID, ActorType: "system", RequestID: ids.New(), Delta: seed, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 20}}})
		return e
	}); err != nil {
		return nil, err
	}
	meta := func(actor string) points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: ids.New()}
	}
	game, play, version, err := createRuleWorkflowNamed(ctx, db, admin, reviewer, meta, gameCode)
	if err != nil {
		return nil, err
	}
	finance := commission.Service{DB: db}
	phase, err := runPeriod(ctx, db, admin, game, play, version, session, betting.Service{DB: db}, finance, 1, "correction-manual")
	if err != nil {
		return nil, err
	}
	if err = setOriginalPaymentGate(ctx, db, admin, meta(adminID)); err != nil {
		return nil, err
	}
	if _, err = finance.ProcessPayments(ctx, 100); err != nil {
		return nil, err
	}
	paymentID, err := paymentForCycle(ctx, db, phase.cycleID)
	if err != nil {
		return nil, err
	}
	var payment commission.Payment
	if err = inTx(ctx, db, func(tx pgx.Tx) error {
		var e error
		payment, e = finance.PaymentTx(ctx, tx, fixtureBrand, paymentID)
		return e
	}); err != nil {
		return nil, err
	}
	if payment.State != "awaiting_approval" || payment.TotalPoints != "1" || payment.PayoutMode != commission.PayoutManual {
		return nil, errors.New("positive manual payout did not await approval for one point")
	}
	return map[string]string{"payment_id": payment.ID, "cycle_id": phase.cycleID, "total_points": payment.TotalPoints, "state": payment.State}, nil
}
