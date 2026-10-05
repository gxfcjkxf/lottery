package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var policyUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type PolicyVersions struct {
	Brand int64 `json:"brand"`
	Game  int64 `json:"game"`
}

type EffectivePolicy struct {
	MinBetPoints        points.Amount  `json:"min_bet_points"`
	MaxBetPoints        *points.Amount `json:"max_bet_points"`
	MaxPeriodPoints     *points.Amount `json:"max_period_points"`
	MaxUserPeriodPoints *points.Amount `json:"max_user_period_points"`
	UserCancelAllowed   bool           `json:"user_cancel_allowed"`
}

type BrandPolicyConfig struct {
	MinBetPoints        points.Amount  `json:"min_bet_points"`
	MaxBetPoints        *points.Amount `json:"max_bet_points"`
	MaxPeriodPoints     *points.Amount `json:"max_period_points"`
	MaxUserPeriodPoints *points.Amount `json:"max_user_period_points"`
	UserCancelAllowed   bool           `json:"user_cancel_allowed"`
}

type LimitOverride struct {
	Mode   string         `json:"mode"`
	Points *points.Amount `json:"points"`
}

type GamePolicyConfig struct {
	MinBetPoints        LimitOverride `json:"min_bet_points"`
	MaxBetPoints        LimitOverride `json:"max_bet_points"`
	MaxPeriodPoints     LimitOverride `json:"max_period_points"`
	MaxUserPeriodPoints LimitOverride `json:"max_user_period_points"`
	UserCancelAllowed   *bool         `json:"user_cancel_allowed"`
}

type BrandPolicyRecord struct {
	BrandID   string            `json:"brand_id"`
	Version   int64             `json:"version"`
	Config    BrandPolicyConfig `json:"config"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type GamePolicyRecord struct {
	BrandID   string           `json:"brand_id"`
	GameID    string           `json:"game_id"`
	Version   int64            `json:"version"`
	Config    GamePolicyConfig `json:"config"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func DefaultBrandPolicy() BrandPolicyConfig {
	return BrandPolicyConfig{MinBetPoints: 1}
}

func inheritLimit() LimitOverride { return LimitOverride{Mode: "inherit"} }

func DefaultGamePolicy() GamePolicyConfig {
	return GamePolicyConfig{
		MinBetPoints: inheritLimit(), MaxBetPoints: inheritLimit(),
		MaxPeriodPoints: inheritLimit(), MaxUserPeriodPoints: inheritLimit(),
	}
}

func validPositiveLimit(v *points.Amount) bool { return v == nil || *v > 0 }

func ValidateBrandPolicy(config BrandPolicyConfig) error {
	if config.MinBetPoints <= 0 || !validPositiveLimit(config.MaxBetPoints) ||
		!validPositiveLimit(config.MaxPeriodPoints) || !validPositiveLimit(config.MaxUserPeriodPoints) {
		return ErrInvalid
	}
	for _, cap := range []*points.Amount{config.MaxBetPoints, config.MaxPeriodPoints, config.MaxUserPeriodPoints} {
		if cap != nil && config.MinBetPoints > *cap {
			return ErrInvalid
		}
	}
	return nil
}

func validateOverride(override LimitOverride, minimum bool) error {
	switch override.Mode {
	case "inherit":
		if override.Points != nil {
			return ErrInvalid
		}
	case "unlimited":
		if minimum || override.Points != nil {
			return ErrInvalid
		}
	case "value":
		if override.Points == nil || *override.Points <= 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func ValidateGamePolicy(config GamePolicyConfig) error {
	if validateOverride(config.MinBetPoints, true) != nil ||
		validateOverride(config.MaxBetPoints, false) != nil ||
		validateOverride(config.MaxPeriodPoints, false) != nil ||
		validateOverride(config.MaxUserPeriodPoints, false) != nil {
		return ErrInvalid
	}
	return nil
}

func resolveLimit(parent *points.Amount, override LimitOverride) *points.Amount {
	copyLimit := func(limit *points.Amount) *points.Amount {
		if limit == nil {
			return nil
		}
		value := *limit
		return &value
	}
	switch override.Mode {
	case "inherit":
		return copyLimit(parent)
	case "unlimited":
		// Game configuration overrides the brand default, including no limit.
		return nil
	case "value":
		return copyLimit(override.Points)
	default:
		return nil
	}
}

func ResolvePolicy(brand BrandPolicyConfig, override GamePolicyConfig) (EffectivePolicy, error) {
	if ValidateBrandPolicy(brand) != nil || ValidateGamePolicy(override) != nil {
		return EffectivePolicy{}, ErrInvalid
	}
	result := EffectivePolicy{
		MinBetPoints:        brand.MinBetPoints,
		MaxBetPoints:        resolveLimit(brand.MaxBetPoints, override.MaxBetPoints),
		MaxPeriodPoints:     resolveLimit(brand.MaxPeriodPoints, override.MaxPeriodPoints),
		MaxUserPeriodPoints: resolveLimit(brand.MaxUserPeriodPoints, override.MaxUserPeriodPoints),
		UserCancelAllowed:   brand.UserCancelAllowed,
	}
	if override.MinBetPoints.Mode == "value" {
		result.MinBetPoints = *override.MinBetPoints.Points
	}
	if override.UserCancelAllowed != nil {
		result.UserCancelAllowed = *override.UserCancelAllowed
	}
	for _, cap := range []*points.Amount{result.MaxBetPoints, result.MaxPeriodPoints, result.MaxUserPeriodPoints} {
		if cap != nil && result.MinBetPoints > *cap {
			return EffectivePolicy{}, ErrInvalid
		}
	}
	return result, nil
}

type policyRow interface{ Scan(...any) error }

func scanBrandPolicy(row policyRow) (BrandPolicyRecord, error) {
	var record BrandPolicyRecord
	var raw []byte
	if err := row.Scan(&record.BrandID, &record.Version, &raw, &record.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BrandPolicyRecord{}, ErrNotFound
		}
		return BrandPolicyRecord{}, err
	}
	if err := json.Unmarshal(raw, &record.Config); err != nil {
		return BrandPolicyRecord{}, ErrInvalid
	}
	if ValidateBrandPolicy(record.Config) != nil || record.Version <= 0 {
		return BrandPolicyRecord{}, ErrInvalid
	}
	return record, nil
}

func scanGamePolicy(row policyRow) (GamePolicyRecord, error) {
	var record GamePolicyRecord
	var raw []byte
	if err := row.Scan(&record.BrandID, &record.GameID, &record.Version, &raw, &record.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GamePolicyRecord{}, ErrNotFound
		}
		return GamePolicyRecord{}, err
	}
	if err := json.Unmarshal(raw, &record.Config); err != nil {
		return GamePolicyRecord{}, ErrInvalid
	}
	if ValidateGamePolicy(record.Config) != nil || record.Version <= 0 {
		return GamePolicyRecord{}, ErrInvalid
	}
	return record, nil
}

const brandPolicyFields = `brand_id::text,version,config,updated_at`
const gamePolicyFields = `brand_id::text,game_id::text,version,config,updated_at`

func (s Service) BrandPolicy(ctx context.Context, brand string) (BrandPolicyRecord, error) {
	if s.DB == nil || !policyUUIDPattern.MatchString(brand) {
		return BrandPolicyRecord{}, ErrInvalid
	}
	return scanBrandPolicy(s.DB.QueryRow(ctx, `SELECT `+brandPolicyFields+` FROM brand_bet_policies WHERE brand_id=$1`, brand))
}

func (s Service) GamePolicy(ctx context.Context, brand, game string) (GamePolicyRecord, error) {
	if s.DB == nil || !policyUUIDPattern.MatchString(brand) || !policyUUIDPattern.MatchString(game) {
		return GamePolicyRecord{}, ErrInvalid
	}
	return scanGamePolicy(s.DB.QueryRow(ctx, `SELECT `+gamePolicyFields+` FROM game_bet_policies WHERE brand_id=$1 AND game_id=$2`, brand, game))
}

func (s Service) LockedPolicy(ctx context.Context, tx pgx.Tx, brand, game string) (EffectivePolicy, PolicyVersions, error) {
	if tx == nil || !policyUUIDPattern.MatchString(brand) || !policyUUIDPattern.MatchString(game) {
		return EffectivePolicy{}, PolicyVersions{}, ErrInvalid
	}
	brandRecord, err := scanBrandPolicy(tx.QueryRow(ctx, `SELECT `+brandPolicyFields+` FROM brand_bet_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if err != nil {
		return EffectivePolicy{}, PolicyVersions{}, err
	}
	gameRecord, err := scanGamePolicy(tx.QueryRow(ctx, `SELECT `+gamePolicyFields+` FROM game_bet_policies WHERE brand_id=$1 AND game_id=$2 FOR SHARE`, brand, game))
	if err != nil {
		return EffectivePolicy{}, PolicyVersions{}, err
	}
	resolved, err := ResolvePolicy(brandRecord.Config, gameRecord.Config)
	if err != nil {
		return EffectivePolicy{}, PolicyVersions{}, err
	}
	return resolved, PolicyVersions{Brand: brandRecord.Version, Game: gameRecord.Version}, nil
}

func validatePolicyWrite(tx pgx.Tx, brand string, account access.Account, version int64, reason string, meta points.Metadata) error {
	if tx == nil || !policyUUIDPattern.MatchString(brand) || version <= 0 || !validPolicyReason(reason) ||
		meta.ActorType != "admin" || !policyUUIDPattern.MatchString(account.ID) || meta.ActorID != account.ID ||
		meta.RequestID == "" || len(meta.RequestID) > 80 || !utf8.ValidString(meta.RequestID) || strings.IndexByte(meta.RequestID, 0) >= 0 {
		return ErrInvalid
	}
	if account.SuperAdmin || account.Type != access.AccountAdmin ||
		!access.Authorize(account, "bet_policy", "write", access.ScopeBrand, brand) {
		return ErrDenied
	}
	return nil
}

func validPolicyReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && utf8.ValidString(reason) && len([]byte(reason)) <= 500 && strings.IndexByte(reason, 0) < 0
}

func WriteBrandPolicy(ctx context.Context, tx pgx.Tx, brand string, account access.Account, version int64, config BrandPolicyConfig, reason string, meta points.Metadata) (BrandPolicyRecord, error) {
	if err := validatePolicyWrite(tx, brand, account, version, reason, meta); err != nil {
		return BrandPolicyRecord{}, err
	}
	if ValidateBrandPolicy(config) != nil {
		return BrandPolicyRecord{}, ErrInvalid
	}
	before, err := scanBrandPolicy(tx.QueryRow(ctx, `SELECT `+brandPolicyFields+` FROM brand_bet_policies WHERE brand_id=$1 FOR UPDATE`, brand))
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	if before.Version != version {
		return BrandPolicyRecord{}, ErrVersion
	}
	if before.Version == math.MaxInt64 {
		return BrandPolicyRecord{}, ErrVersion
	}
	rows, err := tx.Query(ctx, `SELECT config FROM game_bet_policies WHERE brand_id=$1`, brand)
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	for rows.Next() {
		var raw []byte
		var gameConfig GamePolicyConfig
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return BrandPolicyRecord{}, err
		}
		if err = json.Unmarshal(raw, &gameConfig); err != nil {
			rows.Close()
			return BrandPolicyRecord{}, err
		}
		if _, err = ResolvePolicy(config, gameConfig); err != nil {
			rows.Close()
			return BrandPolicyRecord{}, ErrInvalid
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	after, err := scanBrandPolicy(tx.QueryRow(ctx, `UPDATE brand_bet_policies SET version=version+1,config=$3,updated_at=clock_timestamp() WHERE brand_id=$1 AND version=$2 RETURNING `+brandPolicyFields, brand, version, raw))
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	_, err = appendPolicyAudit(ctx, tx, account, brand, "bet_policy.write.brand", brand, reason, meta, before, after)
	if err != nil {
		return BrandPolicyRecord{}, err
	}
	return after, nil
}

func WriteGamePolicy(ctx context.Context, tx pgx.Tx, brand, game string, account access.Account, version int64, config GamePolicyConfig, reason string, meta points.Metadata) (GamePolicyRecord, error) {
	if err := validatePolicyWrite(tx, brand, account, version, reason, meta); err != nil {
		return GamePolicyRecord{}, err
	}
	if !policyUUIDPattern.MatchString(game) || ValidateGamePolicy(config) != nil {
		return GamePolicyRecord{}, ErrInvalid
	}
	var gameExists bool
	err := tx.QueryRow(ctx, `SELECT true FROM games WHERE brand_id=$1 AND id=$2 FOR SHARE`, brand, game).Scan(&gameExists)
	if errors.Is(err, pgx.ErrNoRows) {
		return GamePolicyRecord{}, ErrNotFound
	}
	if err != nil {
		return GamePolicyRecord{}, err
	}
	brandRecord, err := scanBrandPolicy(tx.QueryRow(ctx, `SELECT `+brandPolicyFields+` FROM brand_bet_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if err != nil {
		return GamePolicyRecord{}, err
	}
	if _, err = ResolvePolicy(brandRecord.Config, config); err != nil {
		return GamePolicyRecord{}, ErrInvalid
	}
	before, err := scanGamePolicy(tx.QueryRow(ctx, `SELECT `+gamePolicyFields+` FROM game_bet_policies WHERE brand_id=$1 AND game_id=$2 FOR UPDATE`, brand, game))
	if err != nil {
		return GamePolicyRecord{}, err
	}
	if before.Version != version {
		return GamePolicyRecord{}, ErrVersion
	}
	if before.Version == math.MaxInt64 {
		return GamePolicyRecord{}, ErrVersion
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return GamePolicyRecord{}, err
	}
	after, err := scanGamePolicy(tx.QueryRow(ctx, `UPDATE game_bet_policies SET version=version+1,config=$4,updated_at=clock_timestamp() WHERE brand_id=$1 AND game_id=$2 AND version=$3 RETURNING `+gamePolicyFields, brand, game, version, raw))
	if err != nil {
		return GamePolicyRecord{}, err
	}
	_, err = appendPolicyAudit(ctx, tx, account, brand, "bet_policy.write.game", game, reason, meta, before, after)
	if err != nil {
		return GamePolicyRecord{}, err
	}
	return after, nil
}

func appendPolicyAudit(ctx context.Context, tx pgx.Tx, account access.Account, brand, action, resource, reason string, meta points.Metadata, before, after any) (string, error) {
	return audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: account.ID, Action: action, ResourceType: "bet_policy", ResourceID: resource, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
}
