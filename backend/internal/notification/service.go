// Package notification materializes a durable, in-app-only inbox from committed
// business events. It never changes a wallet or calls an external provider.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Consumer = "in-app-notification-v1"

var ErrInvalid = errors.New("invalid notification input or event")
var ErrNotFound = errors.New("notification not found in scope")
var ErrState = errors.New("notification delivery state changed")
var ErrDenied = errors.New("notification operation denied")
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Service struct{ DB *pgxpool.Pool }
type Payload struct {
	ResourceID string                   `json:"resource_id"`
	Points     *string                  `json:"points"`
	Draw       *DrawNotificationPayload `json:"draw,omitempty"`
}

// DrawNotificationPayload is an immutable published result, not the current
// pointer, an order outcome, a prize credit or an authorization to settle.
type DrawNotificationPayload struct {
	GameID         string     `json:"game_id"`
	PeriodID       string     `json:"period_id"`
	PeriodNo       string     `json:"period_no"`
	Result         rules.Draw `json:"result"`
	DrawnAt        time.Time  `json:"drawn_at"`
	PreviousDrawID *string    `json:"previous_draw_id"`
}
type Item struct {
	ID              string     `json:"id"`
	BrandID         string     `json:"brand_id"`
	MemberID        string     `json:"member_id"`
	EventType       string     `json:"event_type"`
	TemplateKey     string     `json:"template_key"`
	TemplateVersion int64      `json:"template_version"`
	Content         Content    `json:"content"`
	Payload         Payload    `json:"payload"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadAt          *time.Time `json:"read_at"`
}
type Page struct {
	TotalCount  string `json:"total_count"`
	BrandID     string `json:"brand_id"`
	MemberID    string `json:"member_id"`
	Items       []Item `json:"items"`
	UnreadCount string `json:"unread_count"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
}
type ReadReceipt struct {
	BrandID     string   `json:"brand_id"`
	MemberID    string   `json:"member_id"`
	IDs         []string `json:"ids"`
	Changed     int64    `json:"changed"`
	UnreadCount string   `json:"unread_count"`
}
type rowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func list(ctx context.Context, q rowQuery, brand, member string, limit, offset int) (Page, error) {
	out := Page{BrandID: brand, MemberID: member, Items: []Item{}, Limit: limit, Offset: offset}
	if !uuid.MatchString(brand) || !uuid.MatchString(member) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var raw []byte
	// Count and page are one statement, hence one READ COMMITTED snapshot.
	err := q.QueryRow(ctx, `SELECT
 (SELECT count(*)::text FROM notifications WHERE brand_id=$1 AND member_id=$2),
 (SELECT count(*)::text FROM notifications WHERE brand_id=$1 AND member_id=$2 AND read_at IS NULL),
 COALESCE((SELECT jsonb_agg(to_jsonb(n) ORDER BY created_at DESC,id DESC) FROM
 (SELECT id::text,brand_id::text,member_id::text,event_type,template_key,template_version,content,payload,created_at,read_at
 FROM notifications WHERE brand_id=$1 AND member_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4) n),'[]'::jsonb)`, brand, member, limit, offset).Scan(&out.TotalCount, &out.UnreadCount, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &out.Items)
	}
	if err == nil {
		for _, item := range out.Items {
			if item.TemplateKey != item.EventType || !validTemplateVersion(item.TemplateVersion) ||
				ValidateContent(item.EventType, item.Content) != nil {
				return out, ErrInvalid
			}
		}
	}
	return out, err
}
func (s Service) List(ctx context.Context, brand, member string, limit, offset int) (Page, error) {
	return list(ctx, s.DB, brand, member, limit, offset)
}
func (s Service) ListTx(ctx context.Context, tx pgx.Tx, brand, member string, limit, offset int) (Page, error) {
	return list(ctx, tx, brand, member, limit, offset)
}
func ValidIDs(values []string) bool {
	if len(values) < 1 || len(values) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !uuid.MatchString(v) || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func (s Service) MarkRead(ctx context.Context, tx pgx.Tx, brand, member string, values []string) (ReadReceipt, error) {
	out := ReadReceipt{BrandID: brand, MemberID: member, IDs: append([]string(nil), values...)}
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(member) || !ValidIDs(values) {
		return out, ErrInvalid
	}
	// Stable locking order prevents two overlapping read batches from deadlocking.
	rows, err := tx.Query(ctx, `SELECT id FROM notifications WHERE brand_id=$1 AND member_id=$2 AND id=ANY($3::uuid[]) ORDER BY id FOR UPDATE`, brand, member, values)
	if err != nil {
		return out, err
	}
	n := 0
	for rows.Next() {
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if n != len(values) {
		return out, ErrNotFound
	}
	cmd, err := tx.Exec(ctx, `UPDATE notifications SET read_at=clock_timestamp() WHERE brand_id=$1 AND member_id=$2 AND id=ANY($3::uuid[]) AND read_at IS NULL`, brand, member, values)
	if err != nil {
		return out, err
	}
	out.Changed = cmd.RowsAffected()
	err = tx.QueryRow(ctx, `SELECT count(*)::text FROM notifications WHERE brand_id=$1 AND member_id=$2 AND read_at IS NULL`, brand, member).Scan(&out.UnreadCount)
	return out, err
}

// Process delivers a bounded batch of transactionally queued events in independent
// transaction. SKIP LOCKED supports multiple workers; a poison event cannot hold
// the queue hostage. Commit atomically couples content, acknowledgement and state.
func (s Service) Process(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	done := 0
	for i := 0; i < limit; i++ {
		progressed, e := s.processOne(ctx)
		if e != nil {
			return done, e
		}
		if !progressed {
			break
		}
		done++
	}
	return done, nil
}
func (s Service) processOne(ctx context.Context) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, brand, kind, aggregate string
	var raw []byte
	var attempts int
	err = tx.QueryRow(ctx, `SELECT d.event_id::text,d.brand_id::text,e.event_type,e.aggregate_id::text,e.payload,d.attempt_count
 FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id
 WHERE d.status='pending' AND d.next_attempt_at<=clock_timestamp() ORDER BY d.next_attempt_at,d.event_id
 LIMIT 1 FOR UPDATE OF d SKIP LOCKED`).Scan(&id, &brand, &kind, &aggregate, &raw, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT inbox_work`); err != nil {
		return false, err
	}
	member, payload, err := validateEvent(ctx, tx, brand, kind, aggregate, raw)
	if err == nil {
		// Copy the currently committed template under a shared row lock. An
		// operator edit cannot change materialized messages or race this copy.
		template, e := s.TemplateTx(ctx, tx, kind, true)
		err = e
		if err == nil {
			err = ValidateContent(kind, template.Content)
		}
		encoded, e := json.Marshal(payload)
		if err == nil {
			err = e
		}
		if err == nil {
			content, e := json.Marshal(template.Content)
			err = e
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO notifications(id,brand_id,member_id,event_id,event_type,template_key,template_version,payload,content)
   VALUES($1,$2,$3,$4,$5,$5,$6,$7,$8) ON CONFLICT(event_id,member_id) DO NOTHING`, ids.New(), brand, member, id, kind, template.Version, encoded, content)
			}
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO consumed_events(consumer,event_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, Consumer, id)
	}
	if err != nil {
		code := "DELIVERY_UNAVAILABLE"
		terminal := attempts+1 >= 5
		if errors.Is(err, ErrInvalid) {
			code = "INVALID_EVENT"
			terminal = true
		}
		if _, e := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT inbox_work`); e != nil {
			return false, e
		}
		state := "pending"
		if terminal {
			state = "failed"
		}
		if _, e := tx.Exec(ctx, `UPDATE notification_deliveries SET status=$2,attempt_count=attempt_count+1,last_error=$3,next_attempt_at=clock_timestamp()+($4*interval '1 second') WHERE event_id=$1`, id, state, code, retryDelay(attempts)); e != nil {
			return false, e
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE notification_deliveries SET status='sent',attempt_count=attempt_count+1,last_error=NULL,sent_at=clock_timestamp() WHERE event_id=$1`, id); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
func retryDelay(attempts int) int {
	if attempts > 7 {
		return 300
	}
	return 2 << attempts
}
func positive(v *string) bool {
	if v == nil {
		return false
	}
	n, e := strconv.ParseInt(*v, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == *v
}

func exactEventJSONKeys(raw []byte, expected ...string) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	delim, ok := token.(json.Delim)
	if err != nil || !ok || delim != '{' {
		return false
	}
	seen := make(map[string]bool, len(expected))
	for decoder.More() {
		token, err = decoder.Token()
		key, isString := token.(string)
		if err != nil || !isString || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
	}
	token, err = decoder.Token()
	close, ok := token.(json.Delim)
	if err != nil || !ok || close != '}' || len(seen) != len(expected) {
		return false
	}
	for _, key := range expected {
		if !seen[key] {
			return false
		}
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func validateEvent(ctx context.Context, tx pgx.Tx, brand, kind, aggregate string, raw []byte) (string, Payload, error) {
	if kind == "draw.result.published" || kind == "draw.result.corrected" {
		return validateDrawEvent(ctx, tx, brand, kind, aggregate, raw)
	}
	if strings.HasPrefix(kind, "withdrawal.order.") {
		return validateWithdrawalEvent(ctx, tx, brand, kind, aggregate, raw)
	}
	if strings.HasPrefix(kind, "reward.order.") {
		return validateRewardEvent(ctx, tx, brand, kind, aggregate, raw)
	}
	if kind == "commission.paid" || kind == "commission.adjusted" || kind == "commission.corrected" {
		return validateCommissionEvent(ctx, tx, brand, kind, aggregate, raw)
	}
	if kind == "bet.order.placed" || kind == "bet.order.won" {
		return validateConfiguredBetEvent(ctx, tx, brand, kind, aggregate, raw)
	}
	switch kind {
	case "member.joined", "recharge.confirmed":
		if !exactEventJSONKeys(raw, "member_id", "resource_id", "points") {
			return "", Payload{}, ErrInvalid
		}
	default:
		return "", Payload{}, ErrInvalid
	}
	var in struct {
		MemberID              string  `json:"member_id"`
		ResourceID            string  `json:"resource_id"`
		OrderID               string  `json:"order_id"`
		Points                *string `json:"points"`
		Status                string  `json:"status"`
		Version               int64   `json:"version"`
		PeriodID              string  `json:"period_id"`
		CalculationID         string  `json:"calculation_id"`
		PayoutEntryID         string  `json:"payout_entry_id"`
		JobID                 string  `json:"job_id"`
		CorrectionID          string  `json:"correction_id"`
		OriginalPayoutEntryID string  `json:"original_payout_entry_id"`
	}
	if json.Unmarshal(raw, &in) != nil || !uuid.MatchString(brand) || !uuid.MatchString(in.MemberID) || !uuid.MatchString(aggregate) {
		return "", Payload{}, ErrInvalid
	}
	p := Payload{ResourceID: aggregate, Points: in.Points}
	switch kind {
	case "member.joined":
		if in.ResourceID != aggregate || aggregate != in.MemberID || in.Points != nil {
			return "", p, ErrInvalid
		}
		var ok bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=$1 AND id=$2)`, brand, in.MemberID).Scan(&ok)
		if err != nil {
			return "", p, err
		}
		if !ok {
			return "", p, ErrInvalid
		}
	case "recharge.confirmed":
		if in.ResourceID != aggregate || !positive(in.Points) {
			return "", p, ErrInvalid
		}
		var ok bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recharge_orders WHERE brand_id=$1 AND id=$2 AND member_id=$3 AND state='confirmed' AND points=$4::bigint)`, brand, aggregate, in.MemberID, *in.Points).Scan(&ok)
		if err != nil {
			return "", p, err
		}
		if !ok {
			return "", p, ErrInvalid
		}

	}
	return in.MemberID, p, nil
}

type Delivery struct {
	EventID       string     `json:"event_id"`
	BrandID       string     `json:"brand_id"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	LastError     *string    `json:"last_error"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	SentAt        *time.Time `json:"sent_at"`
}

const deliveryFields = `event_id::text,brand_id::text,status,attempt_count,last_error,next_attempt_at,sent_at`

func scanDelivery(r pgx.Row) (Delivery, error) {
	var d Delivery
	e := r.Scan(&d.EventID, &d.BrandID, &d.Status, &d.AttemptCount, &d.LastError, &d.NextAttemptAt, &d.SentAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return d, e
}
func (s Service) Deliveries(ctx context.Context, brand string, limit, offset int) ([]Delivery, error) {
	if !uuid.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, `SELECT `+deliveryFields+` FROM notification_deliveries WHERE brand_id=$1 ORDER BY next_attempt_at DESC,event_id DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s Service) Retry(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, attempts int, reason string, meta points.Metadata) (Delivery, error) {
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(id) || attempts < 1 || utf8.RuneCountInString(reason) > 500 {
		return Delivery{}, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "notification", "retry", access.ScopeBrand, brand) {
		return Delivery{}, ErrDenied
	}
	before, e := scanDelivery(tx.QueryRow(ctx, `SELECT `+deliveryFields+` FROM notification_deliveries WHERE brand_id=$1 AND event_id=$2 FOR UPDATE`, brand, id))
	if e != nil {
		return before, e
	}
	if before.Status != "failed" || before.AttemptCount != attempts {
		return before, ErrState
	}
	after, e := scanDelivery(tx.QueryRow(ctx, `UPDATE notification_deliveries SET status='pending',next_attempt_at=clock_timestamp() WHERE brand_id=$1 AND event_id=$2 RETURNING `+deliveryFields, brand, id))
	if e != nil {
		return after, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "notification.retry", ResourceType: "notification_delivery", ResourceID: id, Reason: strings.TrimSpace(reason), RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	return after, e
}
