package reportarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

const archiveJSON = `jsonb_build_object('id',r.id,'brand_id',r.brand_id,'window',jsonb_build_object('kind',r.kind,'period_key',r.period_key,'timezone',r.timezone,'from',r.from_at,'to',r.to_at),
 'revision',r.revision,'previous_id',r.previous_id,'snapshot_at',r.snapshot_at,'created_by',r.created_by,'reason',r.reason,
 'payload_sha256',r.payload_sha256,'audit_log_id',r.audit_log_id,'created_at',r.created_at,'snapshot',r.payload) || CASE WHEN to_jsonb(r)->>'automatic_task_id' IS NULL THEN '{}'::jsonb ELSE jsonb_build_object('automation',jsonb_build_object('task_id',to_jsonb(r)->'automatic_task_id','policy_version',to_jsonb(r)->'automatic_policy_version')) END`

// CreateTx captures an elapsed period as one immutable observation. It does
// not finalize unsettled orders or make any financial change. The caller owns
// authorization rechecks, idempotency, commit and all error rollbacks.
func (s Service) CreateTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in Input, meta points.Metadata) (Record, error) {
	var out Record
	if tx == nil || !archiveUUID.MatchString(brand) || !validReason(in.Reason) || in.ExpectedRevision < 0 || in.ExpectedRevision >= maxRevision || a.Type != access.AccountAdmin || !archiveUUID.MatchString(a.ID) || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return out, ErrInvalid
	}
	if !Allowed(a, brand, "create") {
		return out, ErrDenied
	}
	// Validate the key before constructing the series lock, without accepting
	// whitespace or an arbitrary caller-supplied interval.
	if _, err := ResolveWindow(in.Kind, in.PeriodKey, "UTC"); err != nil {
		return out, ErrInvalid
	}
	var zone, status string
	err := tx.QueryRow(ctx, `SELECT timezone,status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&zone, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if status == "disabled" {
		return out, ErrState
	}
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended(current_schema()||':report-archive:'||$1||':'||$2||':'||$3,0))`, brand, in.Kind, in.PeriodKey).Scan(&locked); err != nil {
		return out, err
	}
	if !locked {
		return out, ErrBusy
	}
	var raw []byte
	var previous Record
	var previousIntact bool
	err = tx.QueryRow(ctx, `SELECT `+archiveJSON+`,payload_sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex') FROM report_archives r WHERE brand_id=$1 AND kind=$2 AND period_key=$3 ORDER BY revision DESC LIMIT 1`, brand, in.Kind, in.PeriodKey).Scan(&raw, &previousIntact)
	if err == nil {
		if !previousIntact {
			return out, ErrIntegrity
		}
		if err = json.Unmarshal(raw, &previous); err != nil {
			return out, err
		}
		zone = previous.Window.Timezone
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if previous.Revision != in.ExpectedRevision {
		return out, ErrVersion
	}
	window := previous.Window
	if previous.ID == "" {
		window, err = ResolveWindow(in.Kind, in.PeriodKey, zone)
		if err != nil {
			return out, ErrInvalid
		}
	}
	var elapsed bool
	if err = tx.QueryRow(ctx, `SELECT $1::timestamptz<=clock_timestamp()`, window.To).Scan(&elapsed); err != nil {
		return out, err
	}
	if !elapsed {
		return out, ErrState
	}
	id := ids.New()
	revision := previous.Revision + 1
	var previousID *string
	var before any
	if previous.ID != "" {
		previousID = &previous.ID
		before = map[string]any{"id": previous.ID, "revision": previous.Revision, "payload_sha256": previous.PayloadSHA256}
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "report_archive.create", ResourceType: "report_archive", ResourceID: id, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"id": id, "brand_id": brand, "kind": window.Kind, "period_key": window.PeriodKey, "timezone": window.Timezone, "from": window.From, "to": window.To, "revision": revision, "previous_id": previousID}})
	if err != nil {
		return out, err
	}
	// Capture and insert share one statement snapshot and timestamp. The
	// database independently recomputes that same snapshot in its INSERT guard.
	err = tx.QueryRow(ctx, `WITH capture AS MATERIALIZED (SELECT report_archive_capture($2,$6,$7,$5) AS payload), inserted AS (
 INSERT INTO report_archives(id,brand_id,kind,period_key,timezone,from_at,to_at,revision,previous_id,snapshot_at,created_by,reason,request_id,payload,payload_sha256,audit_log_id,created_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,(payload->>'snapshot_at')::timestamptz,$10,$11,$12,payload,encode(sha256(convert_to(payload::text,'UTF8')),'hex'),$13,(payload->>'snapshot_at')::timestamptz FROM capture RETURNING *)
 SELECT `+archiveJSON+` FROM inserted r`, id, brand, window.Kind, window.PeriodKey, window.Timezone, window.From, window.To, revision, previousID, a.ID, in.Reason, meta.RequestID, auditID).Scan(&raw)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (s Service) ReadTx(ctx context.Context, tx pgx.Tx, brand, id string) (Record, error) {
	var out Record
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(id) {
		return out, ErrInvalid
	}
	var raw []byte
	var intact bool
	err := tx.QueryRow(ctx, `SELECT `+archiveJSON+`,payload_sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex') FROM report_archives r WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&raw, &intact)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if !intact {
		return out, ErrIntegrity
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
func (s Service) ListTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (Page, error) {
	out := Page{BrandID: brand, Items: []Record{}, Limit: limit, Offset: offset}
	if tx == nil || !archiveUUID.MatchString(brand) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	var raw []byte
	var intact bool
	err := tx.QueryRow(ctx, `WITH page AS (SELECT * FROM report_archives WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*)::text FROM report_archives WHERE brand_id=$1),coalesce((SELECT jsonb_agg(`+archiveJSON+` ORDER BY r.created_at DESC,r.id DESC) FROM page r),'[]'::jsonb),coalesce((SELECT bool_and(payload_sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex')) FROM page),true)`, brand, limit, offset).Scan(&out.TotalCount, &raw, &intact)
	if err != nil {
		return out, err
	}
	if !intact {
		return out, ErrIntegrity
	}
	err = json.Unmarshal(raw, &out.Items)
	return out, err
}

// CanonicalPayloadTx returns the exact UTF-8 JSONB representation whose bytes
// were sealed. Future downloads must use these bytes rather than re-marshalling
// a Go DTO with a different key order, whitespace or timestamp spelling.
func (s Service) CanonicalPayloadTx(ctx context.Context, tx pgx.Tx, brand, id string) ([]byte, string, error) {
	if tx == nil || !archiveUUID.MatchString(brand) || !archiveUUID.MatchString(id) {
		return nil, "", ErrInvalid
	}
	var raw, expected string
	err := tx.QueryRow(ctx, `SELECT payload::text,payload_sha256 FROM report_archives WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&raw, &expected)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	actual := sha256.Sum256([]byte(raw))
	if hex.EncodeToString(actual[:]) != expected {
		return nil, "", ErrIntegrity
	}
	return []byte(raw), expected, nil
}
