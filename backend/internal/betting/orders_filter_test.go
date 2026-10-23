package betting

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"testing"
)

func TestOrdersGameFilterPreservesBrandAndMemberScope(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	fundBettingWallet(t, f, 100)
	order, e := placeBettingOrder(t, f, f.input, "game-filter-order")
	if e != nil {
		t.Fatal(e)
	}
	emptyGame, foreignGame := ids.New(), ids.New()
	for _, item := range []struct{ id, brand, code string }{{emptyGame, f.brand, "empty_filter"}, {foreignGame, storeTestOtherBrand, "foreign_filter"}} {
		if _, e = f.db.Exec(ctx, `INSERT INTO games(id,brand_id,code,name,model,timezone,created_by) SELECT $1,$2,$3,'Filter test',model,timezone,created_by FROM games WHERE id=$4`, item.id, item.brand, item.code, f.game.ID); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := f.service.Orders(ctx, f.brand, f.member, f.game.ID, 1, 0)
	if e != nil || len(rows) != 1 || rows[0].ID != order.ID {
		t.Fatalf("matching combined filters: %+v %v", rows, e)
	}
	rows, e = f.service.Orders(ctx, f.brand, "", emptyGame, 1, 0)
	if e != nil || len(rows) != 0 {
		t.Fatalf("empty game widened scope: %+v %v", rows, e)
	}
	rows, e = f.service.Orders(ctx, f.brand, ids.New(), f.game.ID, 1, 0)
	if e != nil || len(rows) != 0 {
		t.Fatalf("member filter ignored: %+v %v", rows, e)
	}
	if _, e = f.service.Orders(ctx, f.brand, "", foreignGame, 1, 0); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign game scope: %v", e)
	}
	if _, e = f.service.Orders(ctx, f.brand, "", "bad", 1, 0); !errors.Is(e, ErrInvalid) {
		t.Fatalf("invalid game: %v", e)
	}
	rows, e = f.service.Orders(ctx, f.brand, "", "", 1, 0)
	if e != nil || len(rows) != 1 {
		t.Fatalf("all games: %+v %v", rows, e)
	}
}
