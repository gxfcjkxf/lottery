package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

var ErrAttributesMissing = errors.New("Mark Six annual attributes not configured")

type MarkSixAttributes struct {
	ExpiresAt   time.Time         `json:"expires_at"`
	EffectiveAt time.Time         `json:"effective_at"`
	Zodiac      map[string]string `json:"zodiac"`
	Elements    map[string]string `json:"elements"`
	ID          string            `json:"id"`
	Year        int               `json:"year"`
	Version     int64             `json:"version"`
	Waves       map[string]string `json:"waves"`
}
type MarkSixAttributesInput struct {
	ExpiresAt   time.Time         `json:"expires_at"`
	EffectiveAt time.Time         `json:"effective_at"`
	Zodiac      map[string]string `json:"zodiac"`
	Elements    map[string]string `json:"elements"`
	Year        int               `json:"year"`
	Version     int64             `json:"version"`
	Waves       map[string]string `json:"waves"`
	Reason      string            `json:"reason"`
}

func validateWaves(waves map[string]string) error {
	if len(waves) != 49 || waves["49"] != "green" {
		return ErrInvalid
	}
	for n := 1; n <= 49; n++ {
		switch waves[fmt.Sprintf("%02d", n)] {
		case "red", "blue", "green":
		default:
			return ErrInvalid
		}
	}
	return nil
}
func ReadMarkSixAttributes(ctx context.Context, q queryer, game string, year int) (*MarkSixAttributes, error) {
	var a MarkSixAttributes
	var raw, zodiac, elements []byte
	e := q.QueryRow(ctx, `SELECT id::text,year,version,effective_at,expires_at,zodiac,elements,waves FROM platform_mark_six_attributes WHERE game_id=$1 AND year=$2 ORDER BY version DESC LIMIT 1`, game, year).Scan(&a.ID, &a.Year, &a.Version, &a.EffectiveAt, &a.ExpiresAt, &zodiac, &elements, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(raw, &a.Waves)
	if e == nil {
		e = json.Unmarshal(zodiac, &a.Zodiac)
	}
	if e == nil {
		e = json.Unmarshal(elements, &a.Elements)
	}
	return &a, e
}
func (s Service) WriteMarkSixAttributes(ctx context.Context, tx pgx.Tx, a access.Account, game string, in MarkSixAttributesInput, meta points.Metadata) (MarkSixAttributes, error) {
	var out MarkSixAttributes
	if !Allowed(a, "", "write") {
		return out, ErrDenied
	}
	if in.Year < 1900 || in.Year > 9999 || in.Version < 0 || !textOK(in.Reason, 500) || validateWaves(in.Waves) != nil || validateAttributeMap(in.Zodiac, []string{"rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig"}) != nil || validateAttributeMap(in.Elements, []string{"metal", "wood", "water", "fire", "earth"}) != nil || in.EffectiveAt.IsZero() || !in.ExpiresAt.After(in.EffectiveAt) {
		return out, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, game)
	if e != nil {
		return out, e
	}
	if rules.ValidateMarkSixModel(g.Model) != nil {
		return out, ErrInvalid
	}
	old, e := ReadMarkSixAttributes(ctx, tx, game, in.Year)
	if e != nil {
		return out, e
	}
	version := int64(0)
	if old != nil {
		version = old.Version
	}
	if version != in.Version {
		return out, ErrVersionConflict
	}
	out = MarkSixAttributes{ExpiresAt: in.ExpiresAt, EffectiveAt: in.EffectiveAt, Zodiac: in.Zodiac, Elements: in.Elements, ID: ids.New(), Year: in.Year, Version: version + 1, Waves: in.Waves}
	raw, _ := json.Marshal(in.Waves)
	zodiac, _ := json.Marshal(in.Zodiac)
	elements, _ := json.Marshal(in.Elements)
	if _, e = tx.Exec(ctx, `INSERT INTO platform_mark_six_attributes(id,game_id,year,version,waves,created_by,effective_at,zodiac,elements,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, out.ID, game, out.Year, out.Version, raw, a.ID, in.EffectiveAt, zodiac, elements, in.ExpiresAt); e != nil {
		return out, e
	}
	// Only periods without an attribute snapshot can acquire this first profile.
	// Existing period bindings are never overwritten, even within the same year.
	_, e = tx.Exec(ctx, `INSERT INTO platform_mark_six_period_attributes(period_id,game_id,attribute_id) SELECT p.id,p.game_id,$3 FROM platform_game_periods p WHERE p.game_id=$1 AND p.draw_at >= $2 AND $4=(SELECT a.id FROM platform_mark_six_attributes a WHERE a.game_id=p.game_id AND a.effective_at<=p.draw_at AND a.expires_at>p.draw_at ORDER BY a.effective_at DESC,a.version DESC LIMIT 1) AND NOT EXISTS(SELECT 1 FROM platform_mark_six_period_attributes x WHERE x.period_id=p.id)`, game, out.EffectiveAt, out.ID, out.ID)
	if e != nil {
		return out, e
	}
	return out, record(ctx, tx, a, "", "platform_game.mark_six.attributes", game, in.Reason, meta, out)
}

func ReadPeriodMarkSixAttributes(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, period string) (*MarkSixAttributes, error) {
	var a MarkSixAttributes
	var raw, zodiac, elements []byte
	e := q.QueryRow(ctx, `SELECT a.id::text,a.year,a.version,a.effective_at,a.expires_at,a.zodiac,a.elements,a.waves FROM platform_mark_six_period_attributes p JOIN platform_mark_six_attributes a ON a.id=p.attribute_id WHERE p.period_id=$1`, period).Scan(&a.ID, &a.Year, &a.Version, &a.EffectiveAt, &a.ExpiresAt, &zodiac, &elements, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, ErrAttributesMissing
	}
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(raw, &a.Waves)
	if e == nil {
		e = json.Unmarshal(zodiac, &a.Zodiac)
	}
	if e == nil {
		e = json.Unmarshal(elements, &a.Elements)
	}
	return &a, e
}

func validateAttributeMap(values map[string]string, allowed []string) error {
	if len(values) != 49 {
		return ErrInvalid
	}
	for n := 1; n <= 49; n++ {
		v := values[fmt.Sprintf("%02d", n)]
		valid := false
		for _, a := range allowed {
			if v == a {
				valid = true
				break
			}
		}
		if !valid {
			return ErrInvalid
		}
	}
	return nil
}
func ReadEffectiveMarkSixAttributes(ctx context.Context, q queryer, game string, draw time.Time) (*MarkSixAttributes, error) {
	var id string
	e := q.QueryRow(ctx, `SELECT id::text FROM platform_mark_six_attributes WHERE game_id=$1 AND effective_at<=$2 AND expires_at>$2 ORDER BY effective_at DESC,version DESC LIMIT 1`, game, draw).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, ErrAttributesMissing
	}
	if e != nil {
		return nil, e
	}
	var a MarkSixAttributes
	var raw, zodiac, elements []byte
	e = q.QueryRow(ctx, `SELECT id::text,year,version,effective_at,expires_at,zodiac,elements,waves FROM platform_mark_six_attributes WHERE id=$1`, id).Scan(&a.ID, &a.Year, &a.Version, &a.EffectiveAt, &a.ExpiresAt, &zodiac, &elements, &raw)
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(raw, &a.Waves); e == nil {
		e = json.Unmarshal(zodiac, &a.Zodiac)
	}
	if e == nil {
		e = json.Unmarshal(elements, &a.Elements)
	}
	return &a, e
}
