package betting

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func assertActualCorrectionEvent(t *testing.T, f commissionBatchFixture, target commission.CorrectionExecutionTarget) string {
	t.Helper()
	ctx := context.Background()
	var count int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE brand_id=$1 AND event_type='commission.corrected' AND aggregate_id=$2`, f.betting.brand, target.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if target.DeltaPoints == 0 {
		if count != 0 {
			t.Fatalf("zero witness emitted %d correction events", count)
		}
		return ""
	}
	if count != 1 {
		t.Fatalf("actual nonzero target emitted %d events", count)
	}
	var event string
	var raw []byte
	var valid bool
	var deliveries int
	if err := f.betting.db.QueryRow(ctx, `SELECT e.id::text,e.payload,
 valid_commission_correction_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload),
 (SELECT count(*) FROM notification_deliveries d WHERE d.event_id=e.id AND d.brand_id=e.brand_id)
 FROM outbox_events e WHERE e.brand_id=$1 AND e.event_type='commission.corrected' AND e.aggregate_id=$2`, f.betting.brand, target.ID).Scan(&event, &raw, &valid, &deliveries); err != nil {
		t.Fatal(err)
	}
	if !valid || deliveries != 1 {
		t.Fatalf("posting evidence/delivery invalid: valid=%t deliveries=%d", valid, deliveries)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"member_id": target.MemberID, "resource_id": target.ID, "points": strconv.FormatInt(int64(target.DeltaPoints), 10), "ledger_entry_id": *target.LedgerEntryID, "target_id": target.ID}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("outbox witness=%v want=%v", payload, want)
	}
	// This exact one-microsecond posting window isolates the immutable actual
	// target, even after later corrections make its execution stale. It must
	// not report the original credit, forecasts, or zero head witnesses.
	var posted time.Time
	if err := f.betting.db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE id=$1`, *target.LedgerEntryID).Scan(&posted); err != nil {
		t.Fatal(err)
	}
	x := correctionExecutionRead(t, f, target.ExecutionID)
	q := reporting.CommissionQuery{From: posted, To: posted.Add(time.Microsecond), GroupBy: "cycle", Limit: 20, AgentID: &target.AgentID, MemberID: &target.MemberID, CycleID: &x.CycleID}
	report, err := (reporting.Service{DB: f.betting.db}).Commission(ctx, f.betting.brand, q)
	if err != nil {
		t.Fatal(err)
	}
	credit, debit := "0", "0"
	if target.DeltaPoints > 0 {
		credit = strconv.FormatInt(int64(target.DeltaPoints), 10)
	} else {
		debit = strconv.FormatInt(-int64(target.DeltaPoints), 10)
	}
	totals := reporting.CommissionTotals{EntryCount: "1", PaidEntryCount: "0", PaidPoints: "0", AdjustmentEntryCount: "0", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "0", CorrectionEntryCount: "1", CorrectionCreditPoints: credit, CorrectionDebitPoints: debit, NetPoints: strconv.FormatInt(int64(target.DeltaPoints), 10)}
	if report.Summary != totals || report.TotalGroups != "1" || len(report.Items) != 1 || report.Items[0].Key != x.CycleID || report.Items[0].Totals != totals {
		t.Fatalf("actual posting-time correction report=%+v want=%+v", report, totals)
	}
	if _, err = reporting.CommissionCSV(report); err != nil {
		t.Fatal("real correction CSV rejected:", err)
	}
	return event
}

func TestCommissionCorrectionNotificationActualDebitIsImmutablePrivateAndDoesNotMoveWallet(t *testing.T) {
	t.Parallel()
	f, _, _ := readyAutomaticCorrectionExecutionFixture(t)
	ctx := context.Background()
	x := ensureCorrectionExecution(t, f)
	if x.State != commission.CorrectionExecutionCompleted {
		t.Fatalf("execution=%+v", x)
	}
	target := correctionExecutionTargetsRead(t, f, x.ID).Items[0]
	event := assertActualCorrectionEvent(t, f, target)
	for _, sql := range []string{
		`UPDATE outbox_events SET payload=jsonb_set(payload,'{points}','"-2"'::jsonb) WHERE id=$1`,
		`DELETE FROM outbox_events WHERE id=$1`,
		`INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) SELECT gen_random_uuid(),brand_id,event_type,aggregate_id,payload FROM outbox_events WHERE id=$1`,
	} {
		if _, err := f.betting.db.Exec(ctx, sql, event); err == nil {
			t.Fatalf("accepted forged or mutable correction event: %s", sql)
		}
	}
	var valid bool
	for _, mutation := range []string{
		`jsonb_set(payload,'{points}','"0"'::jsonb)`,
		`jsonb_set(payload,'{points}','"01"'::jsonb)`,
		`jsonb_set(payload,'{points}','"1"'::jsonb)`,
		`jsonb_set(payload,'{member_id}','null'::jsonb)`,
		`jsonb_set(payload,'{resource_id}',to_jsonb(gen_random_uuid()::text))`,
		`payload||'{"private":"bad"}'::jsonb`,
	} {
		if err := f.betting.db.QueryRow(ctx, `SELECT valid_commission_correction_notification_event(brand_id,event_type,aggregate_id,`+mutation+`) FROM outbox_events WHERE id=$1`, event).Scan(&valid); err != nil || valid {
			t.Fatalf("accepted invalid witness %s valid=%t err=%v", mutation, valid, err)
		}
	}
	before := correctionExecutionPostingRead(t, f)
	buckets := correctionExecBuckets(t, f)
	s := notification.Service{DB: f.betting.db}
	if _, err := s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	page, err := s.List(ctx, f.betting.brand, target.MemberID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var item *notification.Item
	for i := range page.Items {
		if page.Items[i].EventType == "commission.corrected" {
			if item != nil {
				t.Fatal("duplicate inbox correction")
			}
			item = &page.Items[i]
		}
	}
	if item == nil || item.Content == nil || item.TemplateVersion != 1 || item.TemplateKey != "commission.corrected" || item.Payload.ResourceID != target.ID || item.Payload.Points == nil || *item.Payload.Points != "-1" {
		t.Fatalf("public historical correction=%+v", item)
	}
	var raw []byte
	if err = f.betting.db.QueryRow(ctx, `SELECT payload FROM notifications WHERE event_id=$1`, event).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var public map[string]string
	if err = json.Unmarshal(raw, &public); err != nil || len(public) != 2 || public["resource_id"] != target.ID || public["points"] != "-1" {
		t.Fatalf("unsafe public payload=%s err=%v", raw, err)
	}
	if _, err = s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, before) {
		t.Fatal("notification consumption changed ledger or financial heads")
	}
	if got := correctionExecBuckets(t, f); !reflect.DeepEqual(got, buckets) {
		t.Fatal("notification consumption changed wallets")
	}
	if _, err = f.betting.db.Exec(ctx, `UPDATE outbox_events SET published_at=clock_timestamp() WHERE id=$1`, event); err != nil {
		t.Fatal("legal publication marker update rejected:", err)
	}
	assertActualCorrectionEvent(t, f, target)
}

func TestCommissionCorrectionNotificationOutboxAndMissingEventFailuresRollbackAllMoney(t *testing.T) {
	t.Parallel()
	f, _, _ := readyAutomaticCorrectionExecutionFixture(t)
	ctx := context.Background()
	before := correctionExecutionPostingRead(t, f)
	buckets := correctionExecBuckets(t, f)
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION reject_correction_event_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='commission.corrected' THEN RAISE EXCEPTION 'owned notification outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_correction_event_test BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_correction_event_test()`); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		_, _ = f.service.ProcessCorrectionExecutions(ctx, 100)
		rows := correctionExecutionsRead(t, f)
		if len(rows.Items) != 1 || rows.Items[0].State != commission.CorrectionExecutionFailed {
			t.Fatalf("financial step did not fail safely: %+v", rows)
		}
		if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, before) {
			t.Fatal("notification failure left a ledger/head")
		}
		if got := correctionExecBuckets(t, f); !reflect.DeepEqual(got, buckets) {
			t.Fatal("notification failure moved wallet points")
		}
		if targets := correctionExecutionTargetsRead(t, f, rows.Items[0].ID); targets.TotalCount != "0" {
			t.Fatalf("failure left financial target: %+v", targets)
		}
		var events, deliveries int
		if err := f.betting.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox_events WHERE event_type='commission.corrected'),(SELECT count(*) FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id WHERE e.event_type='commission.corrected')`).Scan(&events, &deliveries); err != nil || events != 0 || deliveries != 0 {
			t.Fatalf("failure left event/delivery %d/%d err=%v", events, deliveries, err)
		}
		if attempt == 0 {
			if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER reject_correction_event_test ON outbox_events; DROP FUNCTION reject_correction_event_test(); ALTER TABLE commission_correction_execution_targets DISABLE TRIGGER commission_correction_notification`); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := f.betting.db.Exec(ctx, `ALTER TABLE commission_correction_execution_targets ENABLE TRIGGER commission_correction_notification`); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := retryCorrectionExecution(t, f, rows.Items[0], correctionExecutionActor(f, "execute_retry")); err != nil {
			t.Fatal(err)
		}
	}
	x := ensureCorrectionExecution(t, f)
	if x.State != commission.CorrectionExecutionCompleted {
		t.Fatalf("recovered execution=%+v", x)
	}
	assertActualCorrectionEvent(t, f, correctionExecutionTargetsRead(t, f, x.ID).Items[0])
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	assertActualCorrectionEvent(t, f, correctionExecutionTargetsRead(t, f, x.ID).Items[0])
}
