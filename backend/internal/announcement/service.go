package announcement

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

const columns = `id::text,brand_id::text,version,generation,status,config,published_at,updated_at`

func scan(row pgx.Row) (Item, error) {
	var v Item
	var raw []byte
	e := row.Scan(&v.ID, &v.BrandID, &v.Version, &v.Generation, &v.Status, &raw, &v.PublishedAt, &v.UpdatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &v.Config)
		v.State = State(v, time.Now())
	}
	return v, e
}
func List(ctx context.Context, tx pgx.Tx, brand string) ([]Item, error) {
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM announcements WHERE brand_id IS NOT DISTINCT FROM NULLIF($1,'')::uuid ORDER BY (config->>'pinned')::boolean DESC,(config->>'sort')::int,updated_at DESC,id`, brand)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func Save(ctx context.Context, tx pgx.Tx, a access.Account, brand, id string, in Input, meta points.Metadata) (Item, error) {
	assets, e := Validate(&in, brand)
	if e != nil {
		return Item{}, e
	}
	if !Allowed(a, brand, "write") {
		return Item{}, ErrDenied
	}
	if brand != "" {
		var status string
		e = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status)
		if e != nil {
			return Item{}, e
		}
		if status == "disabled" {
			return Item{}, ErrConflict
		}
	}
	for _, target := range in.Config.Targets {
		var ok bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1)`, target).Scan(&ok); e != nil {
			return Item{}, e
		}
		if !ok {
			return Item{}, ErrInvalid
		}
	}
	for _, asset := range assets {
		var ok bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM announcement_assets WHERE id=$1 AND brand_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid)`, asset, brand).Scan(&ok); e != nil {
			return Item{}, e
		}
		if !ok {
			return Item{}, ErrInvalid
		}
	}
	var before *Item
	generation := int64(1)
	var published *time.Time
	if id != "" {
		v, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM announcements WHERE id=$1 AND brand_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid FOR UPDATE`, id, brand))
		if errors.Is(e, pgx.ErrNoRows) {
			return Item{}, ErrNotFound
		}
		if e != nil {
			return Item{}, e
		}
		if v.Version != in.Version {
			return Item{}, ErrConflict
		}
		if v.Status == "published" && in.Status == "draft" {
			return Item{}, ErrInvalid
		}
		before = &v
		generation = v.Generation
		published = v.PublishedAt
		if (in.Status == "published" && published == nil) || in.Remind {
			generation++
			now := time.Now().UTC()
			published = &now
		}
	} else {
		if in.Version != 0 {
			return Item{}, ErrInvalid
		}
		id = ids.New()
	}
	if in.Status == "published" {
		if !Allowed(a, brand, "publish") {
			return Item{}, ErrDenied
		}
		if published == nil {
			now := time.Now().UTC()
			published = &now
		}
	}
	if before != nil && before.Status == "published" && in.Status != "published" && !Allowed(a, brand, "publish") {
		return Item{}, ErrDenied
	}
	if in.Remind && in.Status != "published" {
		return Item{}, ErrInvalid
	}
	raw, _ := json.Marshal(in.Config)
	var out Item
	if before == nil {
		out, e = scan(tx.QueryRow(ctx, `INSERT INTO announcements(id,brand_id,version,generation,status,config,published_at) VALUES($1,NULLIF($2,'')::uuid,1,$3,$4,$5,$6) RETURNING `+columns, id, brand, generation, in.Status, raw, published))
	} else {
		out, e = scan(tx.QueryRow(ctx, `UPDATE announcements SET version=version+1,generation=$3,status=$4,config=$5,published_at=$6,updated_at=now() WHERE id=$1 AND brand_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid RETURNING `+columns, id, brand, generation, in.Status, raw, published))
	}
	if e == nil {
		_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "announcement.save", ResourceType: "announcement", ResourceID: id, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: out})
	}
	return out, e
}

type Subject struct {
	Visitor        string
	Member         string
	Session        string
	BrowserSession string
}

func (s Subject) Owners() []string {
	v := []string{"v:" + s.Visitor}
	if s.Member != "" {
		v = append(v, "m:"+s.Member)
	}
	return v
}
func Sync(ctx context.Context, tx pgx.Tx, brand string, s Subject) error {
	if s.Member == "" {
		return nil
	}
	_, e := tx.Exec(ctx, `INSERT INTO announcement_receipts(brand_id,subject,announcement_id,generation,seen_at,session_key,muted_day)
 SELECT brand_id,$3,announcement_id,generation,seen_at,CASE WHEN session_key=$4 THEN $5 ELSE session_key END,muted_day FROM announcement_receipts WHERE brand_id=$1 AND subject=$2
 ON CONFLICT(brand_id,subject,announcement_id,generation) DO UPDATE SET seen_at=greatest(announcement_receipts.seen_at,excluded.seen_at),session_key=CASE WHEN excluded.seen_at>announcement_receipts.seen_at OR announcement_receipts.seen_at IS NULL THEN excluded.session_key ELSE announcement_receipts.session_key END,muted_day=greatest(announcement_receipts.muted_day,excluded.muted_day)`, brand, "v:"+s.Visitor, "m:"+s.Member, s.BrowserSession, s.Session)
	return e
}
func platformLocation(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	var zone string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id=true FOR SHARE`).Scan(&zone); err != nil {
		return nil, err
	}
	return time.LoadLocation(zone)
}
func Feed(ctx context.Context, tx pgx.Tx, brand string, s Subject) ([]Item, error) {
	loc, e := platformLocation(ctx, tx)
	if e != nil {
		return nil, e
	}
	now := time.Now()

	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM announcements WHERE status='published' AND (brand_id=$1 OR brand_id IS NULL AND (config->'targets'='[]'::jsonb OR config->'targets' @> jsonb_build_array($1::text))) ORDER BY (brand_id IS NULL) DESC,(config->>'pinned')::boolean DESC,(config->>'sort')::int,published_at DESC,id`, brand)
	if e != nil {
		return nil, e
	}
	out := []Item{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		if v.State != "active" || v.Config.Audience == "guest" && s.Member != "" || v.Config.Audience == "member" && s.Member == "" {
			continue
		}
		out = append(out, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		v := &out[i]
		v.Eligible = v.Config.Popup
		var seen *time.Time
		var muted bool
		var session bool
		e = tx.QueryRow(ctx, `SELECT max(seen_at),coalesce(bool_or(muted_day=$5),false),coalesce(bool_or(session_key=$6 OR (subject=$7 AND session_key=$8)),false) FROM announcement_receipts WHERE brand_id=$1 AND subject=ANY($2) AND announcement_id=$3 AND generation=$4`, brand, s.Owners(), v.ID, v.Generation, Day(now, loc), s.Session, "v:"+s.Visitor, s.BrowserSession).Scan(&seen, &muted, &session)
		if e != nil {
			return nil, e
		}
		if muted {
			v.Eligible = false
		}
		if seen != nil {
			switch v.Config.Frequency {
			case "once":
				v.Eligible = false
			case "session":
				v.Eligible = v.Eligible && !session
			case "daily":
				v.Eligible = v.Eligible && Day(*seen, loc) != Day(now, loc)
			case "interval":
				today, _ := time.ParseInLocation("2006-01-02", Day(now, loc), loc)
				last, _ := time.ParseInLocation("2006-01-02", Day(*seen, loc), loc)
				v.Eligible = v.Eligible && !today.Before(last.AddDate(0, 0, v.Config.IntervalDays))
			}
		}
	}
	return out, nil
}

type Receipt struct {
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
}
type ReceiptInput struct {
	Action string    `json:"action"`
	Items  []Receipt `json:"items"`
}

func Record(ctx context.Context, tx pgx.Tx, brand string, s Subject, in ReceiptInput) error {
	if in.Action == "sync" {
		if len(in.Items) != 0 {
			return ErrInvalid
		}
		return Sync(ctx, tx, brand, s)
	}
	if (in.Action != "view" && in.Action != "mute") || len(in.Items) == 0 || len(in.Items) > 100 {
		return ErrInvalid
	}
	if in.Action == "view" && len(in.Items) != 1 {
		return ErrInvalid
	}
	feed, e := Feed(ctx, tx, brand, s)
	if e != nil {
		return e
	}
	loc, e := platformLocation(ctx, tx)
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	valid := map[string]Item{}
	for _, v := range feed {
		valid[v.ID] = v
	}
	for _, r := range in.Items {
		v, ok := valid[r.ID]
		if !ok || v.Generation != r.Generation || !v.Config.Popup || in.Action == "mute" && !v.Config.AllowMute {
			return ErrInvalid
		}
		var seen *time.Time
		var day *string
		if in.Action == "view" {
			seen = &now
		} else {
			d := Day(now, loc)
			day = &d
		}
		for _, owner := range s.Owners() {
			_, e = tx.Exec(ctx, `INSERT INTO announcement_receipts(brand_id,subject,announcement_id,generation,seen_at,session_key,muted_day) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(brand_id,subject,announcement_id,generation) DO UPDATE SET seen_at=coalesce(excluded.seen_at,announcement_receipts.seen_at),session_key=CASE WHEN excluded.seen_at IS NULL THEN announcement_receipts.session_key ELSE excluded.session_key END,muted_day=coalesce(excluded.muted_day,announcement_receipts.muted_day)`, brand, owner, r.ID, r.Generation, seen, s.Session, day)
			if e != nil {
				return e
			}
		}
	}
	return nil
}
