package cash

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Amount identities are global across brands and never recycled, even after
// cancellation. A late payment must not identify a different customer's order.
func reserveUniqueAmount(ctx context.Context, tx pgx.Tx, address string, principal int64) (int64, error) {
	if principal > math.MaxInt64-9999 {
		return 0, ErrLimit
	}
	if err := lock(ctx, tx, "usdt-address:"+address); err != nil {
		return 0, err
	}
	var fee int64
	err := tx.QueryRow(ctx, `SELECT f FROM generate_series(1,9999) f WHERE f<$2 AND NOT EXISTS(SELECT 1 FROM cash_usdt_amounts WHERE address=$1 AND payable=$2+f) AND NOT EXISTS(SELECT 1 FROM cash_orders WHERE kind='deposit' AND currency='USDT' AND destination->>'number'=$1 AND (amount=$2+f OR requested_amount=$2+f)) ORDER BY f LIMIT 1`, address, principal).Scan(&fee)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrLimit
	}
	return fee, err
}

type USDTTransfer struct {
	Transaction     string
	Index           int
	Sender, Address string
	Amount          int64
	Time            time.Time
}

// CreditUSDT accepts only a verified, solidified USDT Transfer event. Callers
// cannot supply a member or brand: the immutable allocation owns that scope.
func (s Service) CreditUSDT(ctx context.Context, tx pgx.Tx, transfer USDTTransfer) (bool, error) {
	if !hashReference(transfer.Transaction) || transfer.Index < 0 || !ValidTRON(transfer.Sender) || !ValidTRON(transfer.Address) || transfer.Amount <= 0 || transfer.Time.IsZero() {
		return false, ErrInvalid
	}
	var brand, member, id string
	err := tx.QueryRow(ctx, `SELECT o.brand_id::text,o.member_id::text,o.id::text FROM cash_usdt_amounts a JOIN cash_orders o ON o.id=a.order_id JOIN brands b ON b.id=o.brand_id WHERE a.address=$1 AND a.payable=$2 AND o.automatic AND o.kind='deposit' AND o.currency='USDT' AND b.status='active' AND o.state IN('pending','submitted','expired') AND $3>=o.created_at AND $3<=o.expires_at`, transfer.Address, transfer.Amount, transfer.Time).Scan(&brand, &member, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Match Advance's lock order. Hash lock serializes automatic and manual use.
	if err = memberLock(ctx, tx, brand, member); err != nil {
		return false, err
	}
	if err = lock(ctx, tx, "rotation:"+brand+":USDT"); err != nil {
		return false, err
	}
	if err = lock(ctx, tx, "usdt-transaction:"+transfer.Transaction); err != nil {
		return false, err
	}
	o, err := ReadOrder(ctx, tx, brand, member, id)
	if err != nil {
		return false, err
	}
	if o.State != "pending" && o.State != "submitted" && o.State != "expired" {
		return false, nil
	}
	var used bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM retention_cash_orders() WHERE currency='USDT' AND kind='deposit' AND state='confirmed' AND reference=$1 AND (event_index=-1 OR event_index=$2))`, transfer.Transaction, transfer.Index).Scan(&used); err != nil {
		return false, err
	}
	if used {
		return false, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE cash_orders SET sender=$2,event_index=$3 WHERE id=$1`, id, transfer.Sender, transfer.Index); err != nil {
		return false, err
	}
	if _, err = s.Advance(ctx, tx, brand, "", id, "confirm", Input{Version: o.Version, Amount: Format(transfer.Amount), Reference: transfer.Transaction}, points.Metadata{ActorType: "system"}); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO cash_usdt_receipts(transaction_id,event_index,order_id,sender,received,block_time) VALUES($1,$2,$3,$4,$5,$6)`, transfer.Transaction, transfer.Index, id, transfer.Sender, transfer.Amount, transfer.Time)
	return err == nil, err
}

type USDTScanner struct {
	DB     *pgxpool.Pool
	Source *TronSource
	Cash   Service
}

func (s USDTScanner) Tick(ctx context.Context) (int, error) {
	// Re-scan the entire order window on every pass; outages and pagination never
	// advance a lossy cursor. Retain a day for delayed chain confirmations.
	rows, err := s.DB.Query(ctx, `SELECT a.address,min(o.created_at) FROM cash_usdt_amounts a JOIN cash_orders o ON o.id=a.order_id WHERE o.state IN('pending','submitted','expired') AND o.expires_at>now()-interval '1 day' GROUP BY a.address`)
	if err != nil {
		return 0, err
	}
	type addressWindow struct {
		address string
		since   time.Time
	}
	windows := []addressWindow{}
	for rows.Next() {
		var w addressWindow
		if err = rows.Scan(&w.address, &w.since); err != nil {
			rows.Close()
			return 0, err
		}
		windows = append(windows, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, w := range windows {
		transfers, e := s.Source.Transfers(ctx, w.address, w.since)
		if e != nil {
			return n, e
		}
		for _, transfer := range transfers {
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				return n, e
			}
			ok, e := s.Cash.CreditUSDT(ctx, tx, transfer)
			if e == nil {
				e = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			if e != nil {
				return n, e
			}
			if ok {
				n++
			}
		}
	}
	return n, nil
}
