package betting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// One store and one immutable ordering for all configured game models.
const historySQL = `WITH history AS (
 SELECT 'configured' AS kind,o.id,o.game_id,g.name AS game_name,p.period_no,o.placed_at
 FROM configured_orders o JOIN platform_games g ON g.id=o.game_id JOIN platform_game_periods p ON p.id=o.period_id
 WHERE o.brand_id=$1 AND o.member_id=$2
) `

type HistoryGame struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type HistoryItem struct {
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	GameID   string    `json:"game_id"`
	GameName string    `json:"game_name"`
	PeriodNo string    `json:"period_no"`
	PlacedAt time.Time `json:"placed_at"`
	Order    any       `json:"order"`
}
type HistoryPage struct {
	TotalCount string        `json:"total_count"`
	BrandID    string        `json:"brand_id"`
	MemberID   string        `json:"member_id"`
	GameID     string        `json:"game_id"`
	Games      []HistoryGame `json:"games"`
	Items      []HistoryItem `json:"items"`
	NextCursor *string       `json:"next_cursor"`
	Limit      int           `json:"limit"`
}
type historyCursor struct {
	Brand  string    `json:"brand"`
	Member string    `json:"member"`
	Game   string    `json:"game"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	ID     string    `json:"id"`
}

func decodeHistoryCursor(raw, brand, member, game string) (historyCursor, error) {
	var c historyCursor
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil {
		return c, ErrInvalid
	}
	if json.Unmarshal(b, &c) != nil || c.Brand != brand || c.Member != member || c.Game != game || c.At.IsZero() || !validIDs(c.ID) {
		return c, ErrInvalid
	}
	if c.Kind != "configured" {
		return c, ErrInvalid
	}
	return c, nil
}

// Authentication and all reads share the caller's repeatable-read transaction.
func HistoryTx(ctx context.Context, tx pgx.Tx, brand, member, game, cursor string, limit int) (HistoryPage, error) {
	out := HistoryPage{BrandID: brand, MemberID: member, GameID: game, Games: []HistoryGame{}, Items: []HistoryItem{}, Limit: limit}
	if !validIDs(brand, member) || game != "" && !validIDs(game) || limit < 1 || limit > 100 {
		return out, ErrInvalid
	}
	if err := tx.QueryRow(ctx, historySQL+`SELECT count(*)::text FROM history WHERE ($3::text='' OR game_id=NULLIF($3,'')::uuid)`, brand, member, game).Scan(&out.TotalCount); err != nil {
		return out, err
	}
	var before *time.Time
	kind, id := "", ""
	if cursor != "" {
		c, e := decodeHistoryCursor(cursor, brand, member, game)
		if e != nil {
			return out, e
		}
		before = &c.At
		kind = c.Kind
		id = c.ID
	}
	rows, e := tx.Query(ctx, historySQL+`SELECT DISTINCT game_id::text,game_name FROM history ORDER BY game_name,game_id::text`, brand, member)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var g HistoryGame
		if e = rows.Scan(&g.ID, &g.Name); e != nil {
			rows.Close()
			return out, e
		}
		out.Games = append(out.Games, g)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, historySQL+`SELECT kind,id::text,game_id::text,game_name,period_no,placed_at FROM history
 WHERE ($3::text='' OR game_id=NULLIF($3,'')::uuid)
 AND ($4::timestamptz IS NULL OR (placed_at,kind,id)<($4,$5,NULLIF($6,'')::uuid))
 ORDER BY placed_at DESC,kind DESC,id DESC LIMIT $7`, brand, member, game, before, kind, id, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var item HistoryItem
		if e = rows.Scan(&item.Kind, &item.ID, &item.GameID, &item.GameName, &item.PeriodNo, &item.PlacedAt); e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		raw, e := json.Marshal(historyCursor{Brand: brand, Member: member, Game: game, At: last.PlacedAt, Kind: last.Kind, ID: last.ID})
		if e != nil {
			return out, e
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &next
	}
	if e = historyPayloads(ctx, tx, brand, member, out.Items); e != nil {
		return out, e
	}
	return out, nil
}

func historyPayloads(ctx context.Context, tx pgx.Tx, brand, member string, items []HistoryItem) error {
	positions := map[string]int{}
	ids := []string{}
	for i, item := range items {
		positions[item.ID] = i
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, configuredOrderSelect+` WHERE o.brand_id=$1 AND o.member_id=$2 AND o.id=ANY($3::uuid[])`, brand, member, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		order, err := scanConfiguredOrder(rows)
		if err != nil {
			return err
		}
		items[positions[order.ID]].Order = order
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		if item.Order == nil {
			return ErrSnapshot
		}
	}
	return nil
}
