package betting

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5"
)

func drawNoticeMessages(t *testing.T, f bettingFixture) []notification.Item {
	t.Helper()
	p, err := (notification.Service{DB: f.db}).List(context.Background(), f.brand, f.member, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	items := []notification.Item{}
	for _, item := range p.Items {
		if strings.HasPrefix(item.EventType, "draw.result.") {
			items = append(items, item)
		}
	}
	return items
}

func waitNoticePeriod(t *testing.T, f bettingFixture) (int64, time.Time) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := (rulebook.Store{DB: f.db}).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		var version int64
		var status string
		var at time.Time
		if err := f.db.QueryRow(ctx, `SELECT version,status,draw_at FROM periods WHERE id=$1`, f.period.ID).Scan(&version, &status, &at); err != nil {
			t.Fatal(err)
		}
		if status == "waiting_draw" {
			return version, at
		}
		if time.Now().After(deadline) {
			t.Fatal("normal period clock did not reach draw time")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func finishNoticeDraw(t *testing.T, f bettingFixture) rulebook.DrawResult {
	t.Helper()
	version, at := waitNoticePeriod(t, f)
	return publicManual(t, f, f.period.ID, version, f.period.PeriodNo, correctedDigits(0, 1, 0), at)
}

func TestDrawNotificationOutboxFailureRollsBackPublicationAndResult(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	ctx := context.Background()
	fundBettingWallet(t, f, 37)
	if _, err := placeBettingOrder(t, f, f.input, "notice-fault-order"); err != nil {
		t.Fatal(err)
	}
	version, at := waitNoticePeriod(t, f)
	before := walletBySource(t, f)
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION fail_draw_notice_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.event_type IN('draw.result.published','draw.result.corrected') THEN RAISE EXCEPTION 'injected draw event failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_draw_notice_test BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION fail_draw_notice_test()`); err != nil {
		t.Fatal(err)
	}
	a := judgeActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand})
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (rulebook.Store{DB: f.db}).ManualDraw(ctx, tx, f.brand, a, f.period.ID, version, f.period.PeriodNo, correctedDigits(0, 1, 0), at, "draw notification fault injection", policyMeta(a.ID))
	_ = tx.Rollback(ctx)
	if err == nil || !strings.Contains(err.Error(), "injected draw event failure") {
		t.Fatal("did not exercise real outbox failure", err)
	}
	var state string
	var pointer *string
	var currentVersion int64
	var results, pubs, recipients, events, audits int
	if err = f.db.QueryRow(ctx, `SELECT p.status,p.draw_result_id::text,p.version,
 (SELECT count(*) FROM draw_results WHERE period_id=p.id),(SELECT count(*) FROM draw_notification_publications WHERE period_id=p.id),
 (SELECT count(*) FROM draw_notification_recipients),(SELECT count(*) FROM outbox_events WHERE event_type LIKE 'draw.result.%'),
 (SELECT count(*) FROM audit_logs WHERE action='draw.lock') FROM periods p WHERE id=$1`, f.period.ID).Scan(&state, &pointer, &currentVersion, &results, &pubs, &recipients, &events, &audits); err != nil {
		t.Fatal(err)
	}
	if state != "waiting_draw" || pointer != nil || currentVersion != version || results+pubs+recipients+events+audits != 0 || walletBySource(t, f) != before {
		t.Fatal("partial draw publication after rollback", state, pointer, results, pubs, recipients, events, audits)
	}
	if _, err = f.db.Exec(ctx, `DROP TRIGGER fail_draw_notice_test ON outbox_events; DROP FUNCTION fail_draw_notice_test()`); err != nil {
		t.Fatal(err)
	}
	actual := publicManual(t, f, f.period.ID, version, f.period.PeriodNo, correctedDigits(0, 1, 0), at)
	s := notification.Service{DB: f.db}
	if _, err = s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	items := drawNoticeMessages(t, f)
	if len(items) != 1 || items[0].Payload.ResourceID != actual.ID || walletBySource(t, f) != before {
		t.Fatal("normal recovery failed", items)
	}
}

func TestDrawNotificationPublicationAcceptsOwnSavepointRowsButRejectsCommittedRows(t *testing.T) {
	f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
	ctx := context.Background()
	fundBettingWallet(t, f, 37)
	if _, err := placeBettingOrder(t, f, f.input, "notice-savepoint-order"); err != nil {
		t.Fatal(err)
	}
	version, at := waitNoticePeriod(t, f)
	a := judgeActor(f)
	a.Roles[0].Permissions = append(a.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand})
	var draw rulebook.DrawResult
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SAVEPOINT normal_http_business`); err != nil {
			return err
		}
		var err error
		draw, err = (rulebook.Store{DB: f.db}).ManualDraw(ctx, tx, f.brand, a, f.period.ID, version, f.period.PeriodNo, correctedDigits(0, 1, 0), at, "savepoint publication", policyMeta(a.ID))
		if err != nil {
			return err
		}
		var current bool
		if err = tx.QueryRow(ctx, `SELECT draw_notice_current_row(xmin) FROM periods WHERE id=$1`, f.period.ID).Scan(&current); err != nil {
			return err
		}
		if !current {
			t.Fatal("own savepoint row not recognized")
		}
		_, err = tx.Exec(ctx, `RELEASE SAVEPOINT normal_http_business`)
		return err
	})
	var current bool
	if err := f.db.QueryRow(ctx, `SELECT draw_notice_current_row(xmin) FROM periods WHERE id=$1`, f.period.ID).Scan(&current); err != nil || current {
		t.Fatal("committed row treated as current publication", current, err)
	}
	if _, err := (notification.Service{DB: f.db}).Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	items := drawNoticeMessages(t, f)
	if len(items) != 1 || items[0].Payload.ResourceID != draw.ID {
		t.Fatal(items)
	}
}

func TestDrawNotificationAudienceIncludesCancelledAndAbnormalOrdersOnce(t *testing.T) {
	for _, state := range []string{"cancelled", "abnormal", "placed_twice"} {
		t.Run(state, func(t *testing.T) {
			f := newBettingFixtureWithWindow(t, storeTestBrand, 2*time.Second, 2100*time.Millisecond)
			ctx := context.Background()
			fundBettingWallet(t, f, 37)
			if state == "cancelled" {
				setUserCancellation(t, f, true)
				f.input.PolicyVersions = ptrPolicyVersions(readBettingPolicyVersions(t, f.service, f.brand, f.game.ID))
			}
			o, err := placeBettingOrder(t, f, f.input, "draw-notice-first")
			if err != nil {
				t.Fatal(err)
			}
			switch state {
			case "cancelled":
				bettingTx(t, f.db, func(tx pgx.Tx) error {
					_, e := f.service.Cancel(ctx, tx, f.brand, f.user, o.ID, o.Version, "cancel before draw", points.Metadata{RequestID: ids.New()})
					return e
				})
			case "abnormal":
				bettingTx(t, f.db, func(tx pgx.Tx) error {
					_, e := f.service.MarkAbnormal(ctx, tx, f.brand, exceptionActor(f), o.ID, o.Version, "manual abnormal before draw", policyMeta(f.version.CreatedBy))
					return e
				})
			case "placed_twice":
				if _, err = placeBettingOrder(t, f, f.input, "draw-notice-second"); err != nil {
					t.Fatal(err)
				}
			}
			draw := finishNoticeDraw(t, f)
			before := walletBySource(t, f)
			s := notification.Service{DB: f.db}
			if _, err = s.Process(ctx, 100); err != nil {
				t.Fatal(err)
			}
			items := drawNoticeMessages(t, f)
			if len(items) != 1 || items[0].EventType != "draw.result.published" || items[0].Payload.ResourceID != draw.ID || items[0].Payload.Points != nil || items[0].Content == nil || items[0].Payload.Draw == nil {
				t.Fatal(items)
			}
			facts := items[0].Payload.Draw
			if facts.GameID != f.game.ID || facts.PeriodID != f.period.ID || facts.PeriodNo != f.period.PeriodNo || !reflect.DeepEqual(facts.Result.Digits, []int{0, 1, 0}) || facts.PreviousDrawID != nil || !facts.DrawnAt.Equal(draw.DrawnAt) {
				t.Fatal(facts)
			}
			var recipients int
			if err = f.db.QueryRow(ctx, `SELECT count(*) FROM draw_notification_recipients WHERE draw_result_id=$1`, draw.ID).Scan(&recipients); err != nil || recipients != 1 {
				t.Fatal(recipients, err)
			}
			if _, err = s.Process(ctx, 100); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(drawNoticeMessages(t, f), items) || walletBySource(t, f) != before {
				t.Fatal("duplicate delivery or changed economic state")
			}
		})
	}
}

func TestDrawNotificationDelayedConsumptionPreservesPublishedCorrectionChain(t *testing.T) {
	f, _, _ := settledCorrectionFixture(t)
	ctx := context.Background()
	var original string
	if err := f.db.QueryRow(ctx, `SELECT draw_result_id::text FROM periods WHERE id=$1`, f.period.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	c := createFixtureCorrection(t, f, correctedDigits(0, 1, 0))
	if c.State != "reversing" {
		t.Fatal(c)
	}
	var publications int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM draw_notification_publications WHERE period_id=$1`, f.period.ID).Scan(&publications); err != nil || publications != 1 {
		t.Fatal("unpublished candidate generated a notice", publications, err)
	}
	if _, err := f.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	before := walletBySource(t, f)
	s := notification.Service{DB: f.db}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Process(ctx, 100); failures <- e }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	items := drawNoticeMessages(t, f)
	if len(items) != 2 {
		t.Fatal(items)
	}
	byEvent := map[string]notification.Item{}
	for _, v := range items {
		byEvent[v.EventType] = v
	}
	old, newer := byEvent["draw.result.published"], byEvent["draw.result.corrected"]
	if old.Payload.Draw == nil || newer.Payload.Draw == nil || old.Payload.ResourceID != original || newer.Payload.ResourceID != c.DrawResultID || newer.Payload.Draw.PreviousDrawID == nil || *newer.Payload.Draw.PreviousDrawID != original || !reflect.DeepEqual(old.Payload.Draw.Result.Digits, []int{1, 2, 1}) || !reflect.DeepEqual(newer.Payload.Draw.Result.Digits, []int{0, 1, 0}) {
		t.Fatal(items)
	}
	encoded, _ := json.Marshal(newer.Payload)
	for _, private := range []string{"audit_log_id", "recipient_id", "publication_id", "member_id", "ledger_entry_id", "reason"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("leaked %s", private)
		}
	}
	if _, err := s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drawNoticeMessages(t, f), items) || walletBySource(t, f) != before {
		t.Fatal("replayed notices changed facts or points")
	}
}

func TestDrawNotificationUnsettledCorrectionPublishesImmediatelyAndOldMessagesStay(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	s := notification.Service{DB: f.db}
	if _, err := s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	old := drawNoticeMessages(t, f)
	if len(old) != 1 {
		t.Fatal(old)
	}
	before := walletBySource(t, f)
	c := createFixtureCorrection(t, f, correctedDigits(0, 1, 0))
	if c.State != "completed" || c.PreviousJobID != nil {
		t.Fatal(c)
	}
	if _, err := s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	all := drawNoticeMessages(t, f)
	if len(all) != 2 {
		t.Fatal(all)
	}
	found := false
	for _, n := range all {
		if n.ID == old[0].ID {
			found = reflect.DeepEqual(n, old[0])
		}
	}
	if !found || walletBySource(t, f) != before {
		t.Fatal("correction replaced old notice or moved points")
	}
}

func TestDrawNotificationEvidenceOutboxAndPayloadAreImmutable(t *testing.T) {
	f, _ := drawnFixtureOrder(t)
	ctx := context.Background()
	s := notification.Service{DB: f.db}
	if _, err := s.Process(ctx, 100); err != nil {
		t.Fatal(err)
	}
	items := drawNoticeMessages(t, f)
	if len(items) != 1 {
		t.Fatal(items)
	}
	var draw, event, recipient string
	if err := f.db.QueryRow(ctx, `SELECT p.draw_result_id::text,e.id::text,r.id::text FROM draw_notification_publications p JOIN draw_notification_recipients r ON r.draw_result_id=p.draw_result_id JOIN outbox_events e ON e.aggregate_id=r.id WHERE p.period_id=$1 AND e.event_type=p.event_type`, f.period.ID).Scan(&draw, &event, &recipient); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE draw_notification_publications SET period_no='forged' WHERE draw_result_id=$1`,
		`DELETE FROM draw_notification_publications WHERE draw_result_id=$1`,
		`UPDATE draw_notification_recipients SET created_at=clock_timestamp() WHERE draw_result_id=$1`,
		`DELETE FROM draw_notification_recipients WHERE draw_result_id=$1`,
		`UPDATE outbox_events SET payload=payload||'{"points":"999"}'::jsonb WHERE id=$2`,
		`DELETE FROM outbox_events WHERE id=$2`,
	} {
		tx, err := f.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		args := []any{draw}
		if strings.Contains(q, "$2") {
			args = append(args, event)
		}
		_, err = tx.Exec(ctx, q, args...)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("evidence mutation accepted: %s", q)
		}
	}
	// An existing recipient cannot be used to manufacture a fresh event later.
	if _, err := f.db.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) SELECT $1,brand_id,event_type,aggregate_id,payload FROM outbox_events WHERE id=$2`, ids.New(), event); err == nil {
		t.Fatal("late notification fabrication accepted")
	}
	// A plausible draw payload must still equal the immutable publication.
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO notifications(id,brand_id,member_id,event_id,event_type,template_key,template_version,payload,content)
 SELECT $1,brand_id,member_id,event_id,event_type,template_key,template_version,jsonb_set(payload,'{draw,result,digits}','[9,9,9]'),content FROM notifications WHERE id=$2`, ids.New(), items[0].ID)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("forged historic numbers accepted")
	}
	if _, err = f.db.Exec(ctx, `UPDATE outbox_events SET published_at=clock_timestamp() WHERE id=$1`, event); err != nil {
		t.Fatal("publication acknowledgement rejected", err)
	}
	var valid bool
	if err = f.db.QueryRow(ctx, `SELECT valid_draw_notification_event(brand_id,event_type,aggregate_id,payload) FROM outbox_events WHERE id=$1`, event).Scan(&valid); err != nil || !valid {
		t.Fatal(valid, err)
	}
	if !reflect.DeepEqual(drawNoticeMessages(t, f), items) {
		t.Fatal("guards changed old inbox")
	}
	_ = recipient
}
