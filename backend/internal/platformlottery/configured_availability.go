package platformlottery

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// ConfiguredOptionStates stores explicit states by play ID and option ID.
type ConfiguredOptionStates map[string]map[string]bool

func (states *ConfiguredOptionStates) UnmarshalJSON(raw []byte) error {
	var values map[string]map[string]*bool
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	if values == nil {
		return ErrInvalid
	}
	out := make(ConfiguredOptionStates, len(values))
	for play, options := range values {
		out[play] = make(map[string]bool, len(options))
		for option, enabled := range options {
			if enabled == nil {
				return ErrInvalid
			}
			out[play][option] = *enabled
		}
	}
	*states = out
	return nil
}

type ConfiguredAvailabilityScope struct {
	Version int64                  `json:"version"`
	Items   ConfiguredOptionStates `json:"items"`
}

type ConfiguredAvailability struct {
	RuleID   string                       `json:"rule_id"`
	Platform ConfiguredAvailabilityScope  `json:"platform"`
	Brand    *ConfiguredAvailabilityScope `json:"brand"`
}

type ConfiguredAvailabilityInput struct {
	RuleID  string                 `json:"rule_id"`
	Version int64                  `json:"version"`
	Items   ConfiguredOptionStates `json:"items"`
}

func ReadConfiguredAvailability(ctx context.Context, q queryer, brand, game, rule string) (ConfiguredAvailability, error) {
	out := ConfiguredAvailability{RuleID: rule, Platform: ConfiguredAvailabilityScope{Items: ConfiguredOptionStates{}}}
	if brand != "" {
		out.Brand = &ConfiguredAvailabilityScope{Items: ConfiguredOptionStates{}}
	}
	rows, err := q.Query(ctx, `SELECT brand_id IS NULL,version,items FROM configured_availability WHERE game_id=$1 AND (brand_id IS NULL OR brand_id=NULLIF($2,'')::uuid)`, game, brand)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var platform bool
		var scope ConfiguredAvailabilityScope
		if err := rows.Scan(&platform, &scope.Version, &scope.Items); err != nil {
			return out, err
		}
		if platform {
			out.Platform = scope
		} else {
			out.Brand = &scope
		}
	}
	return out, rows.Err()
}

func optionDisabled(items ConfiguredOptionStates, play, option string) bool {
	enabled, exists := items[play][option]
	return exists && !enabled
}

func (v ConfiguredAvailability) OptionDisabled(play, option string) bool {
	return optionDisabled(v.Platform.Items, play, option) ||
		(v.Brand != nil && optionDisabled(v.Brand.Items, play, option))
}

func (v ConfiguredAvailability) DisabledOptions() map[string][]string {
	disabled := make(map[string][]string)
	for play, options := range v.Platform.Items {
		for option := range options {
			if v.OptionDisabled(play, option) {
				disabled[play] = append(disabled[play], option)
			}
		}
	}
	if v.Brand != nil {
		for play, options := range v.Brand.Items {
			for option, enabled := range options {
				if _, exists := v.Platform.Items[play][option]; !exists && !enabled {
					disabled[play] = append(disabled[play], option)
				}
			}
		}
	}
	return disabled
}

func (s Service) WriteConfiguredAvailability(ctx context.Context, tx pgx.Tx, actor access.Account, brand, game string, in ConfiguredAvailabilityInput, meta points.Metadata) (ConfiguredAvailability, error) {
	var out ConfiguredAvailability
	action := "write"
	if brand != "" {
		action = "sell"
	}
	if !Allowed(actor, brand, action) {
		return out, ErrDenied
	}
	// Orders take a shared lock on the same game before reading availability.
	if _, err := lockedGame(ctx, tx, game); err != nil {
		return out, err
	}
	if brand != "" {
		var connected bool
		err := tx.QueryRow(ctx, `SELECT enabled FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2 FOR SHARE`, brand, game).Scan(&connected)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrNotFound
		}
		if err != nil {
			return out, err
		}
	}
	rule, err := ReadConfiguredPublication(ctx, tx, game)
	if err != nil {
		return out, err
	}
	if in.RuleID != rule.ID {
		return out, ErrVersionConflict
	}
	out, err = ReadConfiguredAvailability(ctx, tx, brand, game, rule.ID)
	if err != nil {
		return out, err
	}
	scope := &out.Platform
	if brand != "" {
		scope = out.Brand
	}
	if in.Version != scope.Version {
		return out, ErrVersionConflict
	}
	var snapshot rules.ConfiguredSnapshot
	if err := json.Unmarshal(rule.Snapshot, &snapshot); err != nil {
		return out, err
	}
	if !validConfiguredOptionStates(snapshot.Catalog, in.Items) {
		return out, ErrInvalid
	}
	if brand != "" {
		for _, play := range snapshot.Catalog.Plays {
			for _, option := range snapshot.Catalog.Domains[play.Selection.Domain].Options {
				if !optionDisabled(out.Platform.Items, play.ID, option.ID) {
					continue
				}
				previous, hadPrevious := scope.Items[play.ID][option.ID]
				next, hasNext := in.Items[play.ID][option.ID]
				if previous != next || hadPrevious != hasNext {
					return out, ErrVersionConflict
				}
			}
		}
	}
	before := *scope
	scope.Version++
	scope.Items = in.Items
	_, err = tx.Exec(ctx, `INSERT INTO configured_availability (game_id,brand_id,version,items)
		VALUES ($1,NULLIF($2,'')::uuid,$3,$4)
		ON CONFLICT (game_id,brand_id) DO UPDATE SET version=excluded.version,items=excluded.items`,
		game, brand, scope.Version, scope.Items)
	if err == nil {
		err = record(ctx, tx, actor, brand, "configured_availability.write", game, "", meta, before, *scope)
	}
	return out, err
}

func validConfiguredOptionStates(catalog rules.ConfiguredCatalog, states ConfiguredOptionStates) bool {
	if states == nil {
		return false
	}
	plays := make(map[string]string, len(catalog.Plays))
	for _, play := range catalog.Plays {
		plays[play.ID] = play.Selection.Domain
	}
	for id, options := range states {
		domain, exists := plays[id]
		if !exists || len(options) == 0 {
			return false
		}
		matches := 0
		for _, option := range catalog.Domains[domain].Options {
			if _, exists := options[option.ID]; exists {
				matches++
			}
		}
		if matches != len(options) {
			return false
		}
	}
	return true
}
