package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ArchiveSnapshot is the aggregate-only record exported for one consistent
// database snapshot. Its family totals use the live reports' closed types.
type ArchiveSnapshot struct {
	BrandID        string            `json:"brand_id"`
	FormatVersion  int               `json:"format_version"`
	SnapshotAt     time.Time         `json:"snapshot_at"`
	Timezone       string            `json:"timezone"`
	From           time.Time         `json:"from"`
	To             time.Time         `json:"to"`
	Betting        BettingTotals     `json:"betting"`
	Ledger         LedgerTotals      `json:"ledger"`
	WalletSnapshot ArchiveWallet     `json:"wallet_snapshot"`
	Withdrawals    WithdrawalTotals  `json:"withdrawals"`
	Commissions    CommissionTotals  `json:"commissions"`
	Rewards        RewardTotals      `json:"rewards"`
	RewardOrders   RewardOrderTotals `json:"reward_orders"`
}

// ArchiveWallet labels current balances with their observation time. They are
// not balances at the end of the requested reporting interval.
type ArchiveWallet struct {
	AtSnapshot time.Time `json:"at_snapshot"`
	Balances   Balances  `json:"balances"`
}

// CaptureArchive captures the report aggregates under one PostgreSQL
// statement snapshot using the brand's current timezone in the header.
func (s Service) CaptureArchive(ctx context.Context, tx pgx.Tx, brand string, from, to time.Time) (ArchiveSnapshot, error) {
	return s.captureArchive(ctx, tx, brand, from, to, nil)
}

// CaptureArchiveTimezone uses timezone only for the returned header. Source
// facts are selected by the same absolute [from,to) instants either way.
func (s Service) CaptureArchiveTimezone(ctx context.Context, tx pgx.Tx, brand string, from, to time.Time, timezone string) (ArchiveSnapshot, error) {
	if timezone == "" || timezone == "Local" {
		return ArchiveSnapshot{BrandID: brand, FormatVersion: 1, From: from, To: to}, ErrInvalid
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return ArchiveSnapshot{BrandID: brand, FormatVersion: 1, From: from, To: to}, ErrInvalid
	}
	return s.captureArchive(ctx, tx, brand, from, to, &timezone)
}

func (Service) captureArchive(ctx context.Context, tx pgx.Tx, brand string, from, to time.Time, timezone *string) (ArchiveSnapshot, error) {
	out := ArchiveSnapshot{BrandID: brand, FormatVersion: 1, From: from, To: to}
	if tx == nil || !uuid.MatchString(brand) || from.IsZero() || to.IsZero() ||
		!to.After(from) || to.Sub(from) > 93*24*time.Hour ||
		from.Year() < 1 || from.Year() > 9999 || to.Year() < 1 || to.Year() > 9999 {
		return out, ErrInvalid
	}
	var result []byte
	err := tx.QueryRow(ctx, `SELECT report_archive_capture($1,$2,$3,$4)`, brand, databaseBound(from), databaseBound(to), timezone).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && len(result) == 0 {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(result, &out); err != nil {
		return out, err
	}
	if out.BrandID != brand || out.FormatVersion != 1 || out.SnapshotAt.IsZero() || out.Timezone == "" {
		return out, ErrInvalid
	}
	out.SnapshotAt = out.SnapshotAt.UTC()
	out.From, out.To = from, to
	out.WalletSnapshot.AtSnapshot = out.SnapshotAt
	return out, nil
}
