package points

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

// RepairPreview describes a materialized-balance repair, never an economic
// credit or a ledger rewrite. Only an intact ledger can authorize rebuilding.
type RepairPreview struct {
	AccountID     string                       `json:"account_id"`
	MemberID      string                       `json:"member_id"`
	Version       int64                        `json:"version"`
	LedgerVersion int64                        `json:"ledger_version"`
	Actual        map[string]map[string]Amount `json:"actual"`
	Expected      Balance                      `json:"expected"`
	Repairable    bool                         `json:"repairable"`
	Consistent    bool                         `json:"consistent"`
	Issues        []string                     `json:"issues"`
	Token         string                       `json:"token"`
}
type RepairRecord struct {
	ID         string        `json:"id"`
	Preview    RepairPreview `json:"preview"`
	AuditLogID string        `json:"audit_log_id"`
}
type RepairHistory struct {
	ID        string          `json:"id"`
	Version   int64           `json:"version"`
	Before    json.RawMessage `json:"before_snapshot"`
	After     json.RawMessage `json:"after_snapshot"`
	Reason    string          `json:"reason"`
	ActorID   string          `json:"actor_id"`
	RequestID string          `json:"request_id"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s Store) ListRepairs(ctx context.Context, brand, member string, limit, offset int) ([]RepairHistory, error) {
	if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(member) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,version,before_snapshot,after_snapshot,reason,actor_id::text,request_id,created_at FROM point_balance_repairs WHERE brand_id=$1 AND member_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, brand, member, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RepairHistory{}
	for rows.Next() {
		var r RepairHistory
		if err = rows.Scan(&r.ID, &r.Version, &r.Before, &r.After, &r.Reason, &r.ActorID, &r.RequestID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s Store) repairPreview(ctx context.Context, tx pgx.Tx, brand, member string, exclusive bool) (RepairPreview, error) {
	out := RepairPreview{MemberID: member, Actual: map[string]map[string]Amount{}, Issues: []string{}}
	if tx == nil || !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(member) {
		return out, ErrInvalid
	}
	lock := " FOR SHARE"
	if exclusive {
		lock = " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, `SELECT id::text,version FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2`+lock, brand, member).Scan(&out.AccountID, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT source,state,points FROM point_buckets WHERE brand_id=$1 AND account_id=$2 ORDER BY source,state`, brand, out.AccountID)
	if err != nil {
		return out, err
	}
	var actual Balance
	var seen [4][4]bool
	count := 0
	for rows.Next() {
		var source, state string
		var p Amount
		if err = rows.Scan(&source, &state, &p); err != nil {
			rows.Close()
			return out, err
		}
		si, e := SourceIndex(source)
		ti, e2 := StateIndex(state)
		if e != nil || e2 != nil || seen[si][ti] {
			rows.Close()
			return out, ErrCorrupt
		}
		seen[si][ti] = true
		if out.Actual[source] == nil {
			out.Actual[source] = map[string]Amount{}
		}
		out.Actual[source][state] = p
		actual[si][ti] = p
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, entrySelect+` WHERE brand_id=$1 AND account_id=$2 ORDER BY version`, brand, out.AccountID)
	if err != nil {
		return out, err
	}
	originals := map[string]Entry{}
	reversed := map[string]bool{}
	intact := true
	// Include every original immutable entry in the optimistic repair token.
	hashes := []string{}
	for rows.Next() {
		e, e2 := scanEntry(rows)
		if e2 != nil {
			rows.Close()
			return out, e2
		}
		out.LedgerVersion++
		c := Change{BrandID: e.BrandID, MemberID: e.MemberID, EntryType: e.EntryType, ReferenceType: e.ReferenceType, ReferenceID: e.ReferenceID, OperationKey: e.OperationKey, Reason: e.Reason, ActorType: e.ActorType, ActorID: e.ActorID, RequestID: e.RequestID, ReversalOf: e.ReversalOf, Delta: e.Delta, Allocation: e.Allocation}
		h, e2 := changeHash(c)
		hashes = append(hashes, e.ID+":"+e.hash)
		if e2 != nil || h != e.hash || !validChange(c) || e.MemberID != member || e.Version != out.LedgerVersion || e.Before != out.Expected {
			intact = false
		}
		if e.ReversalOf == "" {
			if !allocationMatches(c) {
				intact = false
			}
		} else {
			original, ok := originals[e.ReversalOf]
			neg, e3 := Negate(original.Delta)
			a, _ := json.Marshal(original.Allocation)
			b, _ := json.Marshal(e.Allocation)
			if !ok || e3 != nil || original.ReversalOf != "" || reversed[e.ReversalOf] || neg != e.Delta || string(a) != string(b) {
				intact = false
			}
			reversed[e.ReversalOf] = true
		}
		next, e2 := out.Expected.Apply(e.Delta)
		if e2 != nil {
			intact = false
		} else {
			out.Expected = next
		}
		if e.After != out.Expected {
			intact = false
		}
		originals[e.ID] = e
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if !intact {
		out.Issues = append(out.Issues, "ledger integrity cannot be proven")
	}
	if count != 16 {
		out.Issues = append(out.Issues, "missing balance buckets")
	}
	if out.Version != out.LedgerVersion {
		out.Issues = append(out.Issues, "account version differs from ledger")
	}
	if actual.Validate() != nil || actual != out.Expected {
		out.Issues = append(out.Issues, "materialized balance differs from ledger")
	}
	out.Consistent = len(out.Issues) == 0
	out.Repairable = intact && !out.Consistent
	raw, _ := json.Marshal(struct {
		Brand   string
		Preview RepairPreview
		Hashes  []string
	}{brand, out, hashes})
	sum := sha256.Sum256(raw)
	out.Token = hex.EncodeToString(sum[:])
	return out, nil
}

func (s Store) PreviewRepair(ctx context.Context, brand, member string) (RepairPreview, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return RepairPreview{}, err
	}
	defer tx.Rollback(ctx)
	out, err := s.repairPreview(ctx, tx, brand, member, false)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

// PreviewRepairTx only inspects the wallet under its shared account lock. The
// caller owns the transaction and must never treat this historical observation
// as authorization to repair a subsequently changed account.
func (s Store) PreviewRepairTx(ctx context.Context, tx pgx.Tx, brand, member string) (RepairPreview, error) {
	return s.repairPreview(ctx, tx, brand, member, false)
}

// RepairBalance restores only ledger-derived buckets, under the same exclusive
// account lock as Post. No caller-supplied amount and no business ledger mutation.
func (s Store) RepairBalance(ctx context.Context, tx pgx.Tx, brand, member string, version int64, token, reason string, meta Metadata) (RepairRecord, error) {
	reason = strings.TrimSpace(reason)
	if tx == nil || reason == "" || len(reason) > 500 || !utf8.ValidString(reason) || meta.ActorType != "admin" || !uuidPattern.MatchString(meta.ActorID) || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return RepairRecord{}, ErrInvalid
	}
	out, err := s.repairPreview(ctx, tx, brand, member, true)
	if err != nil {
		return RepairRecord{}, err
	}
	if version != out.Version || token != out.Token {
		return RepairRecord{}, ErrConflict
	}
	if !out.Repairable {
		if out.Consistent {
			return RepairRecord{}, ErrConflict
		}
		return RepairRecord{}, ErrCorrupt
	}
	for si, source := range sourceNames {
		for ti, state := range []string{"available", "manual_frozen", "system_frozen", "withdrawal"} {
			_, err = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,$3,$4,$5) ON CONFLICT(brand_id,account_id,source,state) DO UPDATE SET points=EXCLUDED.points`, brand, out.AccountID, source, state, int64(out.Expected[si][ti]))
			if err != nil {
				return RepairRecord{}, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE point_accounts SET version=$3,updated_at=now() WHERE brand_id=$1 AND id=$2`, brand, out.AccountID, out.LedgerVersion); err != nil {
		return RepairRecord{}, err
	}
	id := ids.New()
	before, _ := json.Marshal(map[string]any{"version": out.Version, "buckets": out.Actual})
	after, _ := json.Marshal(map[string]any{"version": out.LedgerVersion, "buckets": out.Expected})
	_, err = tx.Exec(ctx, `INSERT INTO point_balance_repairs(id,brand_id,account_id,member_id,version,before_snapshot,after_snapshot,reason,actor_id,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, brand, out.AccountID, member, out.LedgerVersion, before, after, reason, meta.ActorID, meta.RequestID)
	if err != nil {
		return RepairRecord{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: meta.ActorID, Action: "points.balance_repair", ResourceType: "point_account", ResourceID: out.AccountID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: json.RawMessage(before), After: map[string]any{"repair_id": id, "version": out.LedgerVersion, "buckets": out.Expected}})
	return RepairRecord{ID: id, Preview: out, AuditLogID: auditID}, err
}
