// Package points is the authoritative, transactional sixteen-bucket ledger.
package points

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict      = errors.New("ledger operation conflict")
	ErrNotFound      = errors.New("point account or ledger entry not found")
	ErrCorrupt       = errors.New("point account does not reconcile")
	uuidPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	operationPattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,200}$`)
)

type Store struct{ DB *pgxpool.Pool }
type Metadata struct{ ActorType, ActorID, RequestID, IP, Reason string }
type Change struct {
	BrandID, MemberID, EntryType, ReferenceType, ReferenceID, OperationKey, Reason, ActorType, ActorID, RequestID, IP string
	Delta                                                                                                             Balance
	Allocation                                                                                                        []Allocation
	ReversalOf                                                                                                        string
}
type Entry struct {
	ID            string       `json:"id"`
	BrandID       string       `json:"brand_id"`
	AccountID     string       `json:"account_id"`
	MemberID      string       `json:"member_id"`
	EntryType     string       `json:"entry_type"`
	ReferenceType string       `json:"reference_type"`
	ReferenceID   string       `json:"reference_id"`
	OperationKey  string       `json:"operation_key"`
	Reason        string       `json:"reason"`
	ActorType     string       `json:"actor_type"`
	ActorID       string       `json:"actor_id"`
	RequestID     string       `json:"request_id"`
	ReversalOf    string       `json:"reversal_of,omitempty"`
	Version       int64        `json:"version"`
	Before        Balance      `json:"before_snapshot"`
	Delta         Balance      `json:"delta_snapshot"`
	After         Balance      `json:"after_snapshot"`
	Allocation    []Allocation `json:"source_allocation"`
	CreatedAt     time.Time    `json:"created_at"`
	hash          string
}
type Wallet struct {
	ID                 string  `json:"-"`
	AccountID          string  `json:"account_id"`
	BrandID            string  `json:"brand_id"`
	MemberID           string  `json:"member_id"`
	Version            int64   `json:"version"`
	DisplayPoints      Amount  `json:"display_points"`
	AvailablePoints    Amount  `json:"available_points"`
	FrozenPoints       Amount  `json:"frozen_points"`
	WithdrawalPoints   Amount  `json:"withdrawal_points"`
	RechargePoints     Amount  `json:"recharge_points"`
	WinningPoints      Amount  `json:"winning_points"`
	GiftPoints         Amount  `json:"gift_points"`
	CommissionPoints   Amount  `json:"commission_points"`
	ManualFrozenPoints Amount  `json:"manual_frozen_points"`
	SystemFrozenPoints Amount  `json:"system_frozen_points"`
	BySource           Balance `json:"by_source"`
}
type query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func makeWallet(brand, member, account string, version int64, b Balance) (Wallet, error) {
	if err := b.Validate(); err != nil {
		return Wallet{}, ErrCorrupt
	}
	w := Wallet{ID: account, AccountID: account, BrandID: brand, MemberID: member, Version: version, BySource: b}
	w.AvailablePoints, _ = b.StateTotal(0)
	w.ManualFrozenPoints, _ = b.StateTotal(1)
	w.SystemFrozenPoints, _ = b.StateTotal(2)
	w.WithdrawalPoints, _ = b.StateTotal(3)
	w.FrozenPoints = w.ManualFrozenPoints + w.SystemFrozenPoints
	w.DisplayPoints = w.AvailablePoints + w.FrozenPoints
	// Source summary fields are the available amounts, not a second balance.
	w.RechargePoints = b[0][0]
	w.WinningPoints = b[1][0]
	w.GiftPoints = b[2][0]
	w.CommissionPoints = b[3][0]
	return w, nil
}
func loadBuckets(ctx context.Context, q query, brand, account string) (Balance, error) {
	var b Balance
	var seen [4][4]bool
	count := 0
	rows, err := q.Query(ctx, `SELECT source,state,points FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, brand, account)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		var source, state string
		var amount int64
		if err = rows.Scan(&source, &state, &amount); err != nil {
			return b, err
		}
		si, e := SourceIndex(source)
		ti, e2 := StateIndex(state)
		if e != nil || e2 != nil || seen[si][ti] {
			return b, ErrCorrupt
		}
		seen[si][ti] = true
		count++
		b[si][ti] = Amount(amount)
	}
	if err = rows.Err(); err != nil {
		return b, err
	}
	if count != 16 {
		return b, ErrCorrupt
	}
	if b.Validate() != nil {
		return b, ErrCorrupt
	}
	return b, nil
}

// Snapshot holds a shared account row lock until its caller's transaction ends.
// All mutation paths take that account's exclusive row lock first.
func (s Store) Snapshot(ctx context.Context, tx pgx.Tx, brand, member string) (Wallet, error) {
	return s.snapshot(ctx, tx, brand, member, false)
}

// LockedSnapshot is used for allocation decisions that will subsequently post.
// Taking the exclusive lock immediately avoids concurrent shared-lock upgrades.
func (s Store) LockedSnapshot(ctx context.Context, tx pgx.Tx, brand, member string) (Wallet, error) {
	return s.snapshot(ctx, tx, brand, member, true)
}
func (s Store) snapshot(ctx context.Context, tx pgx.Tx, brand, member string, exclusive bool) (Wallet, error) {
	if tx == nil || !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(member) {
		return Wallet{}, ErrInvalid
	}
	var account string
	var version int64
	lock := " FOR SHARE"
	if exclusive {
		lock = " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, `SELECT id::text,version FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2`+lock, brand, member).Scan(&account, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Wallet{}, ErrNotFound
	}
	if err != nil {
		return Wallet{}, err
	}
	b, err := loadBuckets(ctx, tx, brand, account)
	if err != nil {
		return Wallet{}, err
	}
	return makeWallet(brand, member, account, version, b)
}
func (s Store) Read(ctx context.Context, brand, member string) (Wallet, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Wallet{}, err
	}
	defer tx.Rollback(ctx)
	w, err := s.Snapshot(ctx, tx, brand, member)
	if err != nil {
		return w, err
	}
	return w, tx.Commit(ctx)
}

const entrySelect = `SELECT id::text,brand_id::text,account_id::text,member_id::text,entry_type,reference_type,coalesce(reference_id::text,''),operation_key,reason,actor_type,coalesce(actor_id::text,''),request_id,coalesce(reversal_of::text,''),version,before_snapshot,delta_snapshot,after_snapshot,source_allocation,created_at,request_hash FROM point_ledger_entries`

func scanEntry(row pgx.Row) (Entry, error) {
	var e Entry
	var before, delta, after, allocation []byte
	err := row.Scan(&e.ID, &e.BrandID, &e.AccountID, &e.MemberID, &e.EntryType, &e.ReferenceType, &e.ReferenceID, &e.OperationKey, &e.Reason, &e.ActorType, &e.ActorID, &e.RequestID, &e.ReversalOf, &e.Version, &before, &delta, &after, &allocation, &e.CreatedAt, &e.hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	for _, item := range []struct {
		raw    []byte
		target any
	}{{before, &e.Before}, {delta, &e.Delta}, {after, &e.After}, {allocation, &e.Allocation}} {
		if json.Unmarshal(item.raw, item.target) != nil {
			return e, ErrCorrupt
		}
	}
	return e, nil
}
func (s Store) Entry(ctx context.Context, tx pgx.Tx, brand, member, id string) (Entry, error) {
	if tx == nil || !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(member) || !uuidPattern.MatchString(id) {
		return Entry{}, ErrInvalid
	}
	return scanEntry(tx.QueryRow(ctx, entrySelect+` WHERE brand_id=$1 AND member_id=$2 AND id=$3`, brand, member, id))
}
func changeHash(c Change) (string, error) {
	c.RequestID = ""
	c.IP = ""
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
func validChange(c Change) bool {
	if !uuidPattern.MatchString(c.BrandID) || !uuidPattern.MatchString(c.MemberID) || !operationPattern.MatchString(c.OperationKey) || !operationPattern.MatchString(c.EntryType) || !operationPattern.MatchString(c.ReferenceType) || len(c.Reason) == 0 || len(c.Reason) > 500 || !utf8.ValidString(c.Reason) || len(c.RequestID) == 0 || len(c.RequestID) > 80 {
		return false
	}
	if c.ActorType != "user" && c.ActorType != "admin" && c.ActorType != "system" {
		return false
	}
	if c.ActorID != "" && !uuidPattern.MatchString(c.ActorID) {
		return false
	}
	if c.ActorType != "system" && c.ActorID == "" {
		return false
	}
	if c.ReferenceID != "" && !uuidPattern.MatchString(c.ReferenceID) {
		return false
	}
	if c.ReversalOf != "" && !uuidPattern.MatchString(c.ReversalOf) {
		return false
	}
	changed := false
	for _, s := range c.Delta {
		for _, p := range s {
			changed = changed || p != 0
		}
	}
	return changed
}
func allocationMatches(c Change) bool {
	if len(c.Allocation) < 1 || len(c.Allocation) > 4 {
		return false
	}
	var seen [4]bool
	for _, a := range c.Allocation {
		si, e := SourceIndex(a.Source)
		state, e2 := StateIndex(a.State)
		if e != nil || e2 != nil || a.Points <= 0 || seen[si] {
			return false
		}
		seen[si] = true
		negative, positive := -1, -1
		for ti, p := range c.Delta[si] {
			if p < 0 {
				if negative >= 0 {
					return false
				}
				negative = ti
			}
			if p > 0 {
				if positive >= 0 {
					return false
				}
				positive = ti
			}
		}
		if negative >= 0 {
			if c.Delta[si][negative] == Amount(math.MinInt64) || -c.Delta[si][negative] != a.Points || state != negative {
				return false
			}
			if positive >= 0 && c.Delta[si][positive] != a.Points {
				return false
			}
		} else {
			if positive < 0 || c.Delta[si][positive] != a.Points || state != positive {
				return false
			}
		}
	}
	for si, s := range c.Delta {
		for _, p := range s {
			if p != 0 && !seen[si] {
				return false
			}
		}
	}
	return true
}

// Post executes inside the enclosing business transaction. The caller must
// propagate every error and roll back; neither a balance nor its ledger may commit alone.
func (s Store) Post(ctx context.Context, tx pgx.Tx, c Change) (Entry, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if tx == nil || !validChange(c) {
		return Entry{}, ErrInvalid
	}
	if c.ReversalOf == "" && !allocationMatches(c) {
		return Entry{}, ErrInvalid
	}
	hash, err := changeHash(c)
	if err != nil {
		return Entry{}, err
	}
	var account string
	var version int64
	err = tx.QueryRow(ctx, `SELECT id::text,version FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE`, c.BrandID, c.MemberID).Scan(&account, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	// Per-account locking serializes both replay checks and the entire ledger chain.
	existing, err := scanEntry(tx.QueryRow(ctx, entrySelect+` WHERE brand_id=$1 AND operation_key=$2`, c.BrandID, c.OperationKey))
	if err == nil {
		if existing.hash != hash {
			return Entry{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Entry{}, err
	}
	before, err := loadBuckets(ctx, tx, c.BrandID, account)
	if err != nil {
		return Entry{}, err
	}
	// Detect out-of-band balance changes before admitting any further transaction.
	if version == 0 {
		if before != (Balance{}) {
			return Entry{}, ErrCorrupt
		}
	} else {
		latest, e := scanEntry(tx.QueryRow(ctx, entrySelect+` WHERE brand_id=$1 AND account_id=$2 AND version=$3`, c.BrandID, account, version))
		if e != nil {
			return Entry{}, ErrCorrupt
		}
		if latest.After != before {
			return Entry{}, ErrCorrupt
		}
	}
	if version == math.MaxInt64 {
		return Entry{}, ErrOverflow
	}
	after, err := before.Apply(c.Delta)
	if err != nil {
		return Entry{}, err
	}
	policy, err := s.LockedPolicy(ctx, tx, c.BrandID)
	if err != nil {
		return Entry{}, err
	}
	if err = policy.CheckBalance(before, after, c.ReversalOf != ""); err != nil {
		return Entry{}, err
	}
	if c.ReversalOf != "" {
		original, e := s.Entry(ctx, tx, c.BrandID, c.MemberID, c.ReversalOf)
		if e != nil {
			return Entry{}, e
		}
		neg, e := Negate(original.Delta)
		if e != nil {
			return Entry{}, e
		}
		origAllocation, _ := json.Marshal(original.Allocation)
		newAllocation, _ := json.Marshal(c.Allocation)
		if neg != c.Delta || original.ReversalOf != "" || string(origAllocation) != string(newAllocation) {
			return Entry{}, ErrInvalid
		}
		var reversed bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_ledger_entries WHERE reversal_of=$1)`, c.ReversalOf).Scan(&reversed); err != nil {
			return Entry{}, err
		}
		if reversed {
			return Entry{}, ErrConflict
		}
	}
	e := Entry{ID: ids.New(), BrandID: c.BrandID, MemberID: c.MemberID, AccountID: account, EntryType: c.EntryType, ReferenceType: c.ReferenceType, ReferenceID: c.ReferenceID, OperationKey: c.OperationKey, Reason: c.Reason, ActorType: c.ActorType, ActorID: c.ActorID, RequestID: c.RequestID, ReversalOf: c.ReversalOf, Version: version + 1, Before: before, Delta: c.Delta, After: after, Allocation: c.Allocation, hash: hash}
	rawBefore, _ := json.Marshal(before)
	rawDelta, _ := json.Marshal(c.Delta)
	rawAfter, _ := json.Marshal(after)
	rawAllocation, err := json.Marshal(c.Allocation)
	if err != nil {
		return Entry{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,reference_id,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,actor_id,request_id,reversal_of,request_hash)
 VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,'')::uuid,$17,NULLIF($18,'')::uuid,$19) RETURNING created_at`, e.ID, e.BrandID, e.AccountID, e.MemberID, e.Version, e.EntryType, e.ReferenceType, e.ReferenceID, e.OperationKey, rawBefore, rawDelta, rawAfter, rawAllocation, e.Reason, e.ActorType, e.ActorID, e.RequestID, e.ReversalOf, hash).Scan(&e.CreatedAt)
	if err != nil {
		var constraint *pgconn.PgError
		if errors.As(err, &constraint) && constraint.Code == "23505" {
			return Entry{}, ErrConflict
		}
		return Entry{}, err
	}
	states := []string{"available", "manual_frozen", "system_frozen", "withdrawal"}
	for si, source := range sourceNames {
		for ti, state := range states {
			if c.Delta[si][ti] == 0 {
				continue
			}
			tag, e2 := tx.Exec(ctx, `UPDATE point_buckets SET points=$4 WHERE brand_id=$1 AND account_id=$2 AND source=$3 AND state=$5`, c.BrandID, account, source, int64(after[si][ti]), state)
			if e2 != nil {
				return Entry{}, e2
			}
			if tag.RowsAffected() != 1 {
				return Entry{}, ErrCorrupt
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE point_accounts SET version=$3,updated_at=now() WHERE brand_id=$1 AND id=$2`, c.BrandID, account, e.Version); err != nil {
		return Entry{}, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: c.BrandID, ActorType: c.ActorType, ActorID: c.ActorID, Action: "points." + c.EntryType, ResourceType: "point_account", ResourceID: account, Reason: c.Reason, RequestID: c.RequestID, IP: c.IP, Before: before, After: map[string]any{"ledger_entry_id": e.ID, "version": e.Version, "balance": after}})
	return e, err
}
func (s Store) Reverse(ctx context.Context, tx pgx.Tx, brand, member, originalID, key, reason string, meta Metadata) (Entry, error) {
	original, err := s.Entry(ctx, tx, brand, member, originalID)
	if err != nil {
		return Entry{}, err
	}
	delta, err := Negate(original.Delta)
	if err != nil {
		return Entry{}, err
	}
	return s.Post(ctx, tx, Change{BrandID: brand, MemberID: member, EntryType: "reversal", ReferenceType: "ledger", ReferenceID: original.ID, OperationKey: key, Reason: reason, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: original.Allocation, ReversalOf: original.ID})
}

type Reconciliation struct {
	Consistent bool     `json:"consistent"`
	AccountID  string   `json:"account_id"`
	MemberID   string   `json:"member_id"`
	Version    int64    `json:"version"`
	EntryCount int64    `json:"entry_count"`
	Expected   Balance  `json:"expected"`
	Actual     Balance  `json:"actual"`
	Issues     []string `json:"issues"`
}

func (s Store) Reconcile(ctx context.Context, brand, member string) (Reconciliation, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Reconciliation{}, err
	}
	defer tx.Rollback(ctx)
	w, err := s.Snapshot(ctx, tx, brand, member)
	if err != nil {
		return Reconciliation{}, err
	}
	out := Reconciliation{AccountID: w.AccountID, MemberID: member, Version: w.Version, Actual: w.BySource, Issues: []string{}}
	rows, err := tx.Query(ctx, entrySelect+` WHERE brand_id=$1 AND account_id=$2 ORDER BY version`, brand, w.AccountID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		e, e2 := scanEntry(rows)
		if e2 != nil {
			rows.Close()
			return out, e2
		}
		out.EntryCount++
		if e.Version != out.EntryCount || e.Before != out.Expected {
			out.Issues = append(out.Issues, "ledger chain is discontinuous")
		}
		c := Change{BrandID: e.BrandID, MemberID: e.MemberID, EntryType: e.EntryType, ReferenceType: e.ReferenceType, ReferenceID: e.ReferenceID, OperationKey: e.OperationKey, Reason: e.Reason, ActorType: e.ActorType, ActorID: e.ActorID, RequestID: e.RequestID, ReversalOf: e.ReversalOf, Delta: e.Delta, Allocation: e.Allocation}
		hash, hashErr := changeHash(c)
		if hashErr != nil || hash != e.hash {
			out.Issues = append(out.Issues, "ledger request hash mismatch")
		}
		next, e2 := out.Expected.Apply(e.Delta)
		if e2 != nil {
			out.Issues = append(out.Issues, "invalid ledger delta")
		} else {
			out.Expected = next
		}
		if e.After != out.Expected {
			out.Issues = append(out.Issues, "ledger after snapshot mismatch")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if out.EntryCount != w.Version {
		out.Issues = append(out.Issues, "account version differs from ledger count")
	}
	if out.Expected != out.Actual {
		out.Issues = append(out.Issues, "account balance differs from ledger reconstruction")
	}
	out.Consistent = len(out.Issues) == 0
	return out, tx.Commit(ctx)
}
func (s Store) List(ctx context.Context, brand, member string, limit, offset int) ([]Entry, error) {
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(member) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, entrySelect+` WHERE brand_id=$1 AND member_id=$2 ORDER BY version DESC LIMIT $3 OFFSET $4`, brand, member, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}
