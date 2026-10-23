package betting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestHistoryCursorScope(t *testing.T) {
	c := historyCursor{Brand: ids.New(), Member: ids.New(), Game: ids.New(), At: time.Now().UTC(), Kind: "standard", ID: ids.New()}
	for _, raw := range []string{"bad", "e30"} {
		if _, e := decodeHistoryCursor(raw, c.Brand, c.Member, c.Game); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
}

func TestHistoryAllStoresPagingFiltersAndScope(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	// Temporary, constraint-free copies isolate synthetic read fixtures from the ledger.
	for _, table := range []string{"bet_orders", "configured_orders", "platform_mark_six_orders", "platform_canada_orders", "games", "periods", "platform_games", "platform_game_periods", "global_users", "platform_mark_six_attributes"} {
		if _, e = tx.Exec(ctx, `CREATE TEMP TABLE `+table+` ON COMMIT DROP AS SELECT * FROM `+table+` WITH NO DATA`); e != nil {
			t.Fatal(e)
		}
	}
	brand, member, otherBrand, otherMember := ids.New(), ids.New(), ids.New(), ids.New()
	game, period, user, attribute := ids.New(), ids.New(), ids.New(), ids.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO games(id,brand_id,name,status) VALUES($1,$2,'Retired named game','disabled')`, game, brand)
	exec(`INSERT INTO periods(id,brand_id,game_id,period_no) VALUES($1,$2,$3,'history-001')`, period, brand, game)
	exec(`INSERT INTO platform_games(id,name,status) VALUES($1,'Retired named game','disabled')`, game)
	exec(`INSERT INTO platform_game_periods(id,game_id,period_no) VALUES($1,$2,'history-001')`, period, game)
	exec(`INSERT INTO global_users(id,username) VALUES($1,'history_member')`, user)
	exec(`INSERT INTO platform_mark_six_attributes(id,year,version,effective_at,expires_at) VALUES($1,2026,1,'2026-01-01','2027-01-01')`, attribute)
	const at = "2026-10-20T12:00:00Z"
	for _, kind := range []string{"standard", "configured", "mark-six", "canada"} {
		for i := 0; i < 3; i++ {
			id := ids.New()
			b, m := brand, member
			if i == 1 {
				b = otherBrand
			}
			if i == 2 {
				m = otherMember
			}
			switch kind {
			case "standard":
				// Another brand needs its own name/period mapping; do not borrow this brand's joins.
				if i == 1 {
					exec(`INSERT INTO games(id,brand_id,name) VALUES($1,$2,'Foreign')`, game, b)
					exec(`INSERT INTO periods(id,brand_id,game_id,period_no) VALUES($1,$2,$3,'foreign')`, period, b, game)
				}
				exec(`INSERT INTO bet_orders(id,brand_id,global_user_id,brand_member_id,account_id,game_id,period_id,play_id,rule_version_id,definition_hash,definition_snapshot,status,version,selection_raw,selection_normalized,expanded_bets,unit_points,combination_count,multiplier,total_points,deduction_allocation,policy_snapshot,brand_policy_version,game_policy_version,debit_entry_id,client_key,placed_at,cancel_reason,prize_points)
     VALUES($1,$2,$3,$4,$3,$5,$6,$3,$3,'hash','{}','placed',1,'{}','{}','[]',1,1,1,9007199254740993,'[]','{}',1,1,$3,'history-key',$7,'',0)`, id, b, user, m, game, period, at)
			case "configured":
				exec(`INSERT INTO configured_orders(id,brand_id,member_id,user_id,game_id,period_id,rule_id,sales_version,snapshot,quote,status,placed_at)
     VALUES($1,$2,$3,$4,$5,$6,$4,1,'{}','{"bet_points":"9007199254740993","lines":[]}','placed',$7)`, id, b, m, user, game, period, at)
			case "mark-six":
				exec(`INSERT INTO platform_mark_six_orders(id,brand_id,member_id,game_id,period_id,rule_version,sales_version,plate,items,total_points,prize_points,refund_points,effective_points,status,placed_at,attribute_id,attribute_snapshot)
     VALUES($1,$2,$3,$4,$5,1,1,'A','[]',9007199254740993,0,0,0,'placed',$6,$7,'{}')`, id, b, m, game, period, at, attribute)
			case "canada":
				exec(`INSERT INTO platform_canada_orders(id,brand_id,member_id,game_id,period_id,rule_version,sales_version,items,total_points,prize_points,refund_points,effective_points,status,placed_at,profile,reserved_points,lost_points)
     VALUES($1,$2,$3,$4,$5,1,1,'[]',9007199254740993,0,0,0,'placed',$6,'canada_niuniu',9007199254740994,0)`, id, b, m, game, period, at)
			}
		}
	}
	seen := map[string]bool{}
	var sequence []string
	cursor := ""
	for pageNo := 0; pageNo < 3; pageNo++ {
		page, e := HistoryTx(ctx, tx, brand, member, "", cursor, 2)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Games) != 1 || page.Games[0].Name != "Retired named game" {
			t.Fatalf("games: %+v", page.Games)
		}
		for _, item := range page.Items {
			if seen[item.Kind] || item.PeriodNo != "history-001" || item.GameName != "Retired named game" || item.Order == nil {
				t.Fatalf("bad item: %+v", item)
			}
			seen[item.Kind] = true
			sequence = append(sequence, item.Kind)
			raw, e := json.Marshal(item.Order)
			if e != nil || !containsHistoryAmount(raw) {
				t.Fatalf("exact amount: %s %v", raw, e)
			}
		}
		if pageNo == 0 {
			if page.NextCursor == nil {
				t.Fatal("missing next page")
			}
			for _, scope := range [][3]string{{otherBrand, member, ""}, {brand, otherMember, ""}, {brand, member, game}} {
				if _, e := HistoryTx(ctx, tx, scope[0], scope[1], scope[2], *page.NextCursor, 2); !errors.Is(e, ErrInvalid) {
					t.Fatal("cursor scope", e)
				}
			}
			// A newer ticket must not shift/duplicate the second page.
			exec(`INSERT INTO configured_orders SELECT * FROM configured_orders WHERE brand_id=$1 AND member_id=$2`, brand, member)
			exec(`UPDATE configured_orders SET id=$1,placed_at='2026-10-21' WHERE ctid=(SELECT ctid FROM configured_orders ORDER BY ctid DESC LIMIT 1)`, ids.New())
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 4 {
		t.Fatal("missing history store", seen)
	}
	if want := []string{"standard", "mark-six", "configured", "canada"}; len(sequence) != len(want) {
		t.Fatal(sequence)
	} else {
		for i := range want {
			if sequence[i] != want[i] {
				t.Fatal("mixed ordering", sequence)
			}
		}
	}
	otherGame, otherPeriod := ids.New(), ids.New()
	exec(`INSERT INTO platform_games(id,name,status) VALUES($1,'Another actual game','offline')`, otherGame)
	exec(`INSERT INTO platform_game_periods(id,game_id,period_no) VALUES($1,$2,'older-period')`, otherPeriod, otherGame)
	exec(`INSERT INTO configured_orders SELECT * FROM configured_orders WHERE brand_id=$1 AND member_id=$2 LIMIT 1`, brand, member)
	exec(`UPDATE configured_orders SET id=$1,game_id=$2,period_id=$3,placed_at='2026-10-18' WHERE ctid=(SELECT ctid FROM configured_orders ORDER BY ctid DESC LIMIT 1)`, ids.New(), otherGame, otherPeriod)
	another, e := HistoryTx(ctx, tx, brand, member, otherGame, "", 10)
	if e != nil || len(another.Items) != 1 || another.Items[0].GameName != "Another actual game" || len(another.Games) != 2 {
		t.Fatal("actual game filter", another, e)
	}
	filtered, e := HistoryTx(ctx, tx, brand, member, game, "", 10)
	if e != nil || len(filtered.Items) != 5 {
		t.Fatalf("filter count %d: %v", len(filtered.Items), e)
	}
	empty, e := HistoryTx(ctx, tx, brand, member, ids.New(), "", 10)
	if e != nil || len(empty.Items) != 0 || len(empty.Games) != 2 {
		t.Fatal(empty, e)
	}
	// The same timestamp and all four payloads survive the round trip without deduping stores.
	for _, item := range filtered.Items {
		if item.GameID != game {
			t.Fatal(item)
		}
	}
}
func containsHistoryAmount(raw []byte) bool {
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	if total, ok := v["total_points"]; ok {
		return total == "9007199254740993"
	}
	q, ok := v["quote"].(map[string]any)
	return ok && q["bet_points"] == "9007199254740993"
}
