package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"

	"github.com/jackc/pgx/v5"
)

func runCommissionPayment(t *testing.T, f commissionBatchFixture, payment commission.Payment) commission.Payment {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
			t.Fatal(err)
		}
		payment = commissionPaymentRead(t, f, payment.ID)
		if payment.State == "paid" {
			return payment
		}
		if payment.State == "failed" {
			t.Fatalf("commission payment failed: %+v", payment)
		}
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("commission payment did not finish: %+v", payment)
	return payment
}

func finishManualCommissionPayment(t *testing.T, f commissionBatchFixture, payment commission.Payment) commission.Payment {
	t.Helper()
	actor := commissionPaymentActor(f)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(context.Background(), tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approve actual commission notification fixture"}, commissionPaymentMeta(actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return runCommissionPayment(t, f, commissionPaymentRead(t, f, payment.ID))
}

func TestCommissionNotificationsFollowOnlyCommittedLedgerAndAdjustments(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	payment := prepareManualCommissionPayment(t, f)
	actor := commissionPaymentActor(f)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approve before injected notification outage"}, commissionPaymentMeta(actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_commission_notification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type IN('commission.paid','commission.adjusted') THEN RAISE EXCEPTION 'test notification outbox unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_notification BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION test_reject_commission_notification()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	var beneficiary string
	if err := f.betting.db.QueryRow(ctx, `SELECT member_id::text FROM commission_earnings WHERE run_id=(SELECT run_id FROM commission_payments WHERE id=$1) LIMIT 1`, payment.ID).Scan(&beneficiary); err != nil {
		t.Fatal(err)
	}
	var targetCount, ledgerCount int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_payment_targets WHERE payment_id=$1 AND state='paid'`, payment.ID).Scan(&targetCount); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission' AND reference_type='commission_payment_target' AND reference_id IN(SELECT id FROM commission_payment_targets WHERE payment_id=$1)`, payment.ID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	failedWallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, beneficiary)
	if err != nil || targetCount != 0 || ledgerCount != 0 || failedWallet.BySource[3][0] != 0 {
		t.Fatalf("failed outbox enqueue did not roll back paid target and points: targets=%d ledger=%d wallet=%+v err=%v", targetCount, ledgerCount, failedWallet, err)
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_commission_notification ON outbox_events; DROP FUNCTION test_reject_commission_notification()`); err != nil {
		t.Fatal(err)
	}
	failed := commissionPaymentRead(t, f, payment.ID)
	if failed.State != "failed" {
		t.Fatalf("outbox failure should fail the payment step for operator retry: %+v", failed)
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.RetryPaymentTx(ctx, tx, f.betting.brand, payment.ID, actor,
			commission.RetryCycleInput{Version: failed.Version, Reason: "retry after notification outbox recovery"}, commissionPaymentMeta(actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	paid := runCommissionPayment(t, f, commissionPaymentRead(t, f, payment.ID))
	if paid.State != "paid" {
		t.Fatalf("recovered payment did not finish: payment=%+v", paid)
	}

	targetPage := commissionTargets(t, f, payment.ID)
	if len(targetPage.Items) != 1 {
		t.Fatalf("paid target page=%+v", targetPage)
	}
	target := targetPage.Items[0]
	if target.State != "paid" || target.MemberID == "" {
		t.Fatalf("paid target missing beneficiary: %+v", target)
	}
	memberWallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, target.MemberID)
	if err != nil || memberWallet.BySource[3][0] != 1 {
		t.Fatalf("commission target wallet=%+v err=%v", memberWallet, err)
	}
	first, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), 1, 3)
	if err != nil || first.DeltaPoints != 2 {
		t.Fatalf("positive adjustment=%+v err=%v", first, err)
	}
	second, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), first.Version, 0)
	if err != nil || second.DeltaPoints != -3 {
		t.Fatalf("negative adjustment=%+v err=%v", second, err)
	}
	var eventID, eventType, aggregate string
	var payload []byte
	if err := f.betting.db.QueryRow(ctx, `SELECT id::text,event_type,aggregate_id::text,payload FROM outbox_events WHERE event_type='commission.paid' AND aggregate_id=$1`, target.ID).Scan(&eventID, &eventType, &aggregate, &payload); err != nil {
		t.Fatal(err)
	}
	for label, forged := range map[string]string{
		"wrong member": `jsonb_set($4::jsonb,'{member_id}',to_jsonb($5::text))`,
		"wrong amount": `jsonb_set($4::jsonb,'{points}',to_jsonb($5::text))`,
		"wrong ledger": `jsonb_set($4::jsonb,'{ledger_entry_id}',to_jsonb($5::text))`,
	} {
		var valid bool
		var err error
		forgedValue := ids.New()
		if label == "wrong amount" {
			forgedValue = "2"
		}
		err = f.betting.db.QueryRow(ctx, `SELECT valid_commission_notification_event($1,$2,$3,`+forged+`)`, f.betting.brand, eventType, aggregate, payload, forgedValue).Scan(&valid)
		if err != nil || valid {
			t.Fatalf("forged %s commission event passed evidence validation: valid=%t err=%v", label, valid, err)
		}
	}

	s := notification.Service{DB: f.betting.db}
	for i := 0; i < 10; i++ {
		done, err := s.Process(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		if done == 0 {
			break
		}
	}
	page, err := s.List(ctx, f.betting.brand, target.MemberID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var commissionItems []notification.Item
	for _, item := range page.Items {
		if item.EventType == "commission.paid" || item.EventType == "commission.adjusted" {
			commissionItems = append(commissionItems, item)
		}
	}
	if len(commissionItems) != 3 {
		t.Fatalf("expected one payment and two adjustment notifications exactly once, got %+v", commissionItems)
	}
	got := map[string]bool{}
	resourceByFact := map[string]string{}
	for _, item := range commissionItems {
		if len(item.Payload.ResourceID) != 36 || item.Payload.Points == nil {
			t.Fatalf("public commission payload leaked or omitted facts: %+v", item.Payload)
		}
		fact := item.EventType + ":" + *item.Payload.Points
		got[fact] = true
		resourceByFact[fact] = item.Payload.ResourceID
	}
	for _, want := range []string{"commission.paid:1", "commission.adjusted:2", "commission.adjusted:-3"} {
		if !got[want] {
			t.Fatalf("missing commission history %q in %v", want, got)
		}
	}
	for fact, resource := range map[string]string{
		"commission.paid:1":      target.ID,
		"commission.adjusted:2":  first.ID,
		"commission.adjusted:-3": second.ID,
	} {
		if resourceByFact[fact] != resource {
			t.Fatalf("public commission resource for %s=%s, want event aggregate %s", fact, resourceByFact[fact], resource)
		}
	}
	memberWallet, err = (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, target.MemberID)
	if err != nil || memberWallet.BySource[3][0] != 0 {
		t.Fatalf("net adjustments should return C available to zero: %+v err=%v", memberWallet, err)
	}
}
