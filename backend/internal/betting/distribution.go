package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"slices"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type DistributionNumber struct {
	Number int    `json:"number"`
	Wave   string `json:"wave"`
	Zodiac string `json:"zodiac"`
	Stake  string `json:"stake"`
	Profit string `json:"profit"`
}
type BetDistribution struct {
	GameID     string               `json:"game_id"`
	PeriodNo   string               `json:"period_no"`
	TotalStake string               `json:"total_stake"`
	Numbers    []DistributionNumber `json:"numbers"`
}

const distributionModel = `(g.model->>'model')='X_PLUS_Y' AND (g.model->>'regular_count')::int=6 AND (g.model->>'special_count')::int=1 AND (g.model->'special_pool'->>'min')::int=1 AND (g.model->'special_pool'->>'max')::int=49`

func (s Service) DistributionGames(ctx context.Context, brand string) ([]ConfiguredGameSummary, error) {
	rows, err := s.DB.Query(ctx, `SELECT g.id::text,g.name FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1 AND g.status<>'deleted' AND `+distributionModel+` ORDER BY g.name,g.id`, brand)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfiguredGameSummary{}
	for rows.Next() {
		var g ConfiguredGameSummary
		if err = rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s Service) BetDistribution(ctx context.Context, brand, game, periodNo string) (BetDistribution, error) {
	out := BetDistribution{GameID: game, Numbers: []DistributionNumber{}}
	if !validIDs(brand, game) {
		return out, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1 AND g.id=$2 AND g.status<>'deleted' AND `+distributionModel+`)`, brand, game).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	var period string
	// Prefer the most recent period that has opened; a future period is selected
	// only when the game has no opened periods yet. Historical lookup is exact.
	err = tx.QueryRow(ctx, `SELECT id::text,period_no FROM platform_game_periods WHERE game_id=$1 AND ($2='' OR period_no=$2) ORDER BY (bet_start_at<=now()) DESC,CASE WHEN bet_start_at<=now() THEN bet_start_at END DESC,bet_start_at ASC LIMIT 1`, game, periodNo).Scan(&period, &out.PeriodNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	sales, err := platformlottery.ReadConfiguredSales(ctx, tx, brand, game, false)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT snapshot,quote FROM configured_orders WHERE brand_id=$1 AND game_id=$2 AND period_id=$3 AND status IN ('placed','settled') ORDER BY id`, brand, game, period)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	total := new(big.Int)
	stakes, returns := map[int]*big.Rat{}, map[int]*big.Int{}
	for n := 1; n <= 49; n++ {
		stakes[n] = new(big.Rat)
		returns[n] = new(big.Int)
	}
	engines := map[string]*rules.ConfiguredEngine{}
	var attributes map[int]distributionAttribute
	for rows.Next() {
		var raw []byte
		var quote rules.ConfiguredQuote
		if err = rows.Scan(&raw, &quote); err != nil {
			return out, err
		}
		engine := engines[string(raw)]
		if engine == nil {
			var snap rules.ConfiguredSnapshot
			if err = json.Unmarshal(raw, &snap); err != nil {
				return out, err
			}
			numberAttributes, e := distributionAttributes(snap.Bindings)
			if e != nil {
				return out, e
			}
			if attributes != nil && !reflect.DeepEqual(attributes, numberAttributes) {
				return out, ErrSnapshot
			}
			attributes = numberAttributes
			engine, err = rules.NewConfiguredEngine(snap.Catalog, snap.Bindings, sales.Prices)
			if err != nil {
				return out, ErrSnapshot
			}
			engines[string(raw)] = engine
		}
		for _, line := range quote.Lines {
			projected, e := engine.ProjectSpecialLine(ctx, line)
			if e != nil {
				return out, ErrSnapshot
			}
			if len(projected) == 0 {
				continue
			}
			wins := 0
			for _, v := range projected {
				if v.Won {
					wins++
				}
			}
			if wins == 0 {
				return out, ErrSnapshot
			}
			total.Add(total, big.NewInt(int64(line.Stake)))
			share := big.NewRat(int64(line.Stake), int64(wins))
			for _, v := range projected {
				if v.Number < 1 || v.Number > 49 {
					return out, ErrSnapshot
				}
				if v.Won {
					stakes[v.Number].Add(stakes[v.Number], share)
				}
				returns[v.Number].Add(returns[v.Number], big.NewInt(int64(v.Return)))
			}
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	// Without purchases, display the effective rules' number attributes.
	if attributes == nil {
		version, e := platformlottery.ReadConfiguredPublication(ctx, tx, game)
		if e != nil {
			return out, e
		}
		var snap rules.ConfiguredSnapshot
		if e = json.Unmarshal(version.Snapshot, &snap); e != nil {
			return out, e
		}
		attributes, e = distributionAttributes(snap.Bindings)
		if e != nil {
			return out, e
		}
	}
	out.TotalStake = distributionPoints(new(big.Rat).SetInt(total))
	for n := 1; n <= 49; n++ {
		profit := new(big.Int).Sub(total, returns[n])
		out.Numbers = append(out.Numbers, DistributionNumber{Number: n, Wave: attributes[n].Wave, Zodiac: attributes[n].Zodiac, Stake: distributionPoints(stakes[n]), Profit: distributionPoints(new(big.Rat).SetInt(profit))})
	}
	return out, tx.Commit(ctx)
}

// Aggregate exact fractions before rounding the display to hundredths of a point.
func distributionPoints(v *big.Rat) string {
	return new(big.Rat).Quo(v, big.NewRat(int64(points.Scale), 1)).FloatString(2)
}

type distributionAttribute struct{ Wave, Zodiac string }

func distributionAttributes(bindings rules.ConfiguredBindings) (map[int]distributionAttribute, error) {
	out := map[int]distributionAttribute{}
	for n := 1; n <= 49; n++ {
		var a distributionAttribute
		for label, numbers := range bindings.Attributes["wave"] {
			if slices.Contains(numbers, n) {
				if a.Wave != "" {
					return nil, ErrSnapshot
				}
				a.Wave = label
			}
		}
		for label, numbers := range bindings.Attributes["zodiac"] {
			if slices.Contains(numbers, n) {
				if a.Zodiac != "" {
					return nil, ErrSnapshot
				}
				a.Zodiac = label
			}
		}
		if !slices.Contains([]string{"red", "blue", "green"}, a.Wave) || !slices.Contains([]string{"rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig"}, a.Zodiac) {
			return nil, ErrSnapshot
		}
		out[n] = a
	}
	return out, nil
}
