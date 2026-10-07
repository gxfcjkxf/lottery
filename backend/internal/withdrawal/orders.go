package withdrawal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderService owns the financial workflow, with no default eligibility adapter.
// The HTTP transport separately authenticates sessions and same-origin intents.
type OrderService struct {
	DB          *pgxpool.Pool
	Points      points.Store
	Eligibility EligibilityChecker
}

const orderColumns = `id::text,brand_id::text,member_id::text,account_id::text,points,state,version,source_allocation,policy_snapshot,eligibility_evidence,cycle_from_at,cycle_from_version,reserve_version,reserve_entry_id::text,coalesce(release_entry_id::text,''),coalesce(paid_entry_id::text,''),created_at,updated_at,reviewed_at,completed_at,decision_reason`

func scanOrder(r row) (o Order, err error) {
	var allocation, policy []byte
	var amount int64
	err = r.Scan(&o.ID, &o.BrandID, &o.MemberID, &o.AccountID, &amount, &o.State, &o.Version, &allocation, &policy, &o.EligibilityEvidence, &o.CycleFromAt, &o.CycleFromVersion, &o.ReserveVersion, &o.ReserveEntryID, &o.ReleaseEntryID, &o.PaidEntryID, &o.CreatedAt, &o.UpdatedAt, &o.ReviewedAt, &o.CompletedAt, &o.DecisionReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	o.Points = points.Amount(amount)
	if json.Unmarshal(allocation, &o.SourceAllocation) != nil || json.Unmarshal(policy, &o.PolicySnapshot) != nil {
		return o, points.ErrCorrupt
	}
	o.CreatedAt = o.CreatedAt.UTC()
	o.UpdatedAt = o.UpdatedAt.UTC()
	return o, nil
}

func validOrderMeta(meta points.Metadata, actorType string) bool {
	return meta.ActorType == actorType && validIDs(meta.ActorID) && len(meta.RequestID) > 0 && len(meta.RequestID) <= 80 && utf8.ValidString(meta.RequestID)
}
func receiptHash(action, id string, input any) (string, error) {
	raw, err := json.Marshal(struct {
		Action string
		ID     string
		Input  any
	}{action, id, input})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func lockReceipt(ctx context.Context, tx pgx.Tx, brand, key string, meta points.Metadata) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "withdrawal-operation:"+brand+":"+meta.ActorType+":"+meta.ActorID+":"+key)
	return err
}
func readReceipt(ctx context.Context, tx pgx.Tx, brand, key, hash string, meta points.Metadata) (Order, bool, error) {
	var raw []byte
	var stored string
	var out Order
	err := tx.QueryRow(ctx, `SELECT request_hash,response FROM withdrawal_operation_receipts WHERE brand_id=$1 AND actor_type=$2 AND actor_id=$3 AND client_key=$4`, brand, meta.ActorType, meta.ActorID, key).Scan(&stored, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if stored != hash {
		return out, true, points.ErrConflict
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, true, points.ErrCorrupt
	}
	return out, true, nil
}
func saveReceipt(ctx context.Context, tx pgx.Tx, o Order, key, hash string, meta points.Metadata) error {
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO withdrawal_operation_receipts(id,brand_id,order_id,actor_type,actor_id,client_key,request_hash,response) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), o.BrandID, o.ID, meta.ActorType, meta.ActorID, key, hash, raw)
	return err
}
func (s OrderService) orderTx(ctx context.Context, tx pgx.Tx, brand, id string, lock bool) (Order, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	out, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM withdrawal_orders WHERE brand_id=$1 AND id=$2`+suffix, brand, id))
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT audit_log_id::text FROM withdrawal_order_transitions WHERE order_id=$1 AND version=$2`, id, out.Version).Scan(&out.AuditLogID)
	}
	return out, err
}
func (s OrderService) Read(ctx context.Context, brand, id string) (Order, error) {
	if s.DB == nil || !validIDs(brand, id) {
		return Order{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(ctx)
	out, err := s.orderTx(ctx, tx, brand, id, false)
	if err != nil {
		return Order{}, err
	}
	return out, tx.Commit(ctx)
}

func orderEvidence(ctx context.Context, tx pgx.Tx, o *Order, from, reason string, meta points.Metadata) error {
	before := map[string]any{"state": from, "version": o.Version - 1}
	id, err := audit.Append(ctx, tx, audit.Record{BrandID: o.BrandID, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "withdrawal." + o.State, ResourceType: "withdrawal_order", ResourceID: o.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"state": o.State, "version": o.Version, "points": o.Points, "reserve_entry_id": o.ReserveEntryID, "release_entry_id": o.ReleaseEntryID, "paid_entry_id": o.PaidEntryID}})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO withdrawal_order_transitions(id,brand_id,order_id,version,from_state,to_state,reason,actor_type,actor_id,audit_log_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10,$11)`, ids.New(), o.BrandID, o.ID, o.Version, from, o.State, reason, meta.ActorType, meta.ActorID, id, o.UpdatedAt)
	if err == nil {
		o.AuditLogID = id
		err = appendOrderEvent(ctx, tx, *o)
	}
	return err
}

// Create requires explicit source allocations and a server-owned qualification
// checker. A browser cannot supply a boolean qualification or evidence object.
// The allocation is not a new implicit withdrawal deduction-priority policy.
func (s OrderService) Create(ctx context.Context, tx pgx.Tx, brand, member string, in OrderInput, meta points.Metadata) (Order, error) {
	var empty Order
	if tx == nil || !validIDs(brand, member) || !validOrderMeta(meta, "user") || ValidateOrderInput(in) != nil {
		return empty, ErrInvalid
	}
	brand = strings.ToLower(brand)
	member = strings.ToLower(member)
	meta.ActorID = strings.ToLower(meta.ActorID)
	if err := lockReceipt(ctx, tx, brand, in.ClientKey, meta); err != nil {
		return empty, err
	}
	var status, userID, userStatus, memberStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	} else if err != nil {
		return empty, err
	}
	if status == "disabled" {
		return empty, ErrDenied
	}
	err := tx.QueryRow(ctx, `SELECT bm.global_user_id::text,bm.status,gu.status FROM brand_members bm JOIN global_users gu ON gu.id=bm.global_user_id WHERE bm.brand_id=$1 AND bm.id=$2 FOR SHARE OF bm,gu`, brand, member).Scan(&userID, &memberStatus, &userStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	if userID != meta.ActorID || memberStatus != "normal" || userStatus != "active" {
		return empty, ErrDenied
	}
	hash, err := receiptHash("create", member, in)
	if err != nil {
		return empty, err
	}
	if out, ok, e := readReceipt(ctx, tx, brand, in.ClientKey, hash, meta); e != nil || ok {
		return out, e
	}
	if s.Eligibility == nil {
		return empty, ErrEligibilityNotConfigured
	}
	policy, err := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if err != nil {
		return empty, err
	}
	if !policy.Config.Enabled || in.Points < policy.Config.MinPoints || policy.Config.MaxPoints != nil && in.Points > *policy.Config.MaxPoints {
		return empty, ErrIneligible
	}
	allowed := map[string]bool{}
	for _, src := range policy.Config.AllowedSources {
		allowed[src] = true
	}
	for _, a := range in.SourceAllocation {
		if !allowed[a.Source] {
			return empty, ErrIneligible
		}
	}
	// Incomplete configured compliance adapters cannot be bypassed by a synthetic
	// turnover checker. No real verification is claimed when checks are disabled.
	var complianceRaw []byte
	if err = tx.QueryRow(ctx, `SELECT config FROM brand_compliance_policies WHERE brand_id=$1 FOR SHARE`, brand).Scan(&complianceRaw); err != nil {
		return empty, err
	}
	var complianceConfig compliance.Config
	if err = json.Unmarshal(complianceRaw, &complianceConfig); err != nil {
		return empty, err
	}
	decision, _, err := compliance.Evaluate(complianceConfig)
	if err != nil {
		return empty, err
	}
	if decision != "allow" {
		return empty, ErrIneligible
	}
	if _, err = s.Points.LockedPolicy(ctx, tx, brand); err != nil {
		return empty, err
	}
	wallet, err := s.Points.LockedSnapshot(ctx, tx, brand, member)
	if err != nil {
		return empty, err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM withdrawal_orders WHERE brand_id=$1 AND member_id=$2 AND state IN('reviewing','processing'))`, brand, member).Scan(&active); err != nil {
		return empty, err
	}
	if active {
		return empty, ErrActiveOrder
	}
	delta, err := points.AllocationDelta(in.SourceAllocation, "available", "withdrawal")
	if err != nil {
		return empty, err
	}
	if _, err = wallet.BySource.Apply(delta); err != nil {
		return empty, err
	}
	var cycleAt *time.Time
	var cycleVersion int64
	err = tx.QueryRow(ctx, `SELECT cutoff_at,cutoff_version FROM withdrawal_turnover_cycles WHERE brand_id=$1 AND member_id=$2`, brand, member).Scan(&cycleAt, &cycleVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	var cutoff time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&cutoff); err != nil {
		return empty, err
	}
	cutoff = cutoff.UTC()
	base, err := turnoverBaseSnapshot(wallet)
	if err != nil {
		return empty, err
	}
	check := EligibilityInput{BrandID: brand, MemberID: member, Policy: policy, Wallet: wallet, TurnoverBase: base, Points: in.Points, SourceAllocation: in.SourceAllocation, CycleFromAt: cycleAt, CycleFromVersion: cycleVersion, CutoffAt: cutoff, CutoffVersion: wallet.Version}
	qualification, err := s.Eligibility.Check(ctx, tx, check)
	if err != nil {
		return empty, err
	}
	if !qualification.Allowed {
		return empty, ErrIneligible
	}
	var evidence map[string]json.RawMessage
	if len(qualification.Evidence) > 16384 || json.Unmarshal(qualification.Evidence, &evidence) != nil || evidence == nil || len(evidence) == 0 {
		return empty, ErrInvalid
	}
	// Preserve checker evidence, but the reserved basis key always comes from
	// the service's pre-reservation snapshot, never the checker or browser.
	baseRaw, err := json.Marshal(base)
	if err != nil {
		return empty, err
	}
	evidence["turnover_base_snapshot"] = baseRaw
	qualificationRaw, err := json.Marshal(evidence)
	if err != nil || len(qualificationRaw) > 16384 {
		return empty, ErrInvalid
	}
	// PostgreSQL's canonical JSONB rendering adds whitespace. Enforce the same
	// persisted limit before any reservation, including the new snapshot bytes.
	var evidenceBytes int
	if err = tx.QueryRow(ctx, `SELECT octet_length($1::jsonb::text)`, qualificationRaw).Scan(&evidenceBytes); err != nil {
		return empty, err
	}
	if evidenceBytes > 16384 {
		return empty, ErrInvalid
	}
	out := Order{ID: ids.New(), BrandID: brand, MemberID: member, AccountID: wallet.AccountID, Points: in.Points, State: "reviewing", Version: 1, SourceAllocation: in.SourceAllocation, PolicySnapshot: policy, EligibilityEvidence: qualificationRaw, CycleFromAt: cycleAt, CycleFromVersion: cycleVersion, CreatedAt: cutoff, UpdatedAt: cutoff}
	entry, err := s.Points.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "withdrawal_reserve", ReferenceType: "withdrawal", ReferenceID: out.ID, OperationKey: "withdrawal-reserve:" + out.ID, Reason: "withdrawal application", ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: out.SourceAllocation})
	if err != nil {
		return empty, err
	}
	out.ReserveEntryID = entry.ID
	out.ReserveVersion = entry.Version
	allocation, _ := json.Marshal(out.SourceAllocation)
	policyRaw, _ := json.Marshal(policy)
	_, err = tx.Exec(ctx, `INSERT INTO withdrawal_orders(id,brand_id,member_id,account_id,points,state,version,source_allocation,policy_snapshot,eligibility_evidence,cycle_from_at,cycle_from_version,reserve_version,reserve_entry_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'reviewing',1,$6,$7,$8,$9,$10,$11,$12,$13,$13)`, out.ID, brand, member, out.AccountID, int64(out.Points), allocation, policyRaw, out.EligibilityEvidence, cycleAt, cycleVersion, out.ReserveVersion, out.ReserveEntryID, out.CreatedAt)
	if err != nil {
		return empty, err
	}
	if err = orderEvidence(ctx, tx, &out, "", "withdrawal application", meta); err != nil {
		return empty, err
	}
	if policy.Config.ReviewMode == "automatic" {
		systemMeta := points.Metadata{ActorType: "system", RequestID: meta.RequestID, IP: meta.IP}
		out, err = s.advanceLocked(ctx, tx, out, "approve", "automatic review by saved policy", systemMeta)
		if err != nil {
			return empty, err
		}
	}
	if err = saveReceipt(ctx, tx, out, in.ClientKey, hash, meta); err != nil {
		return empty, err
	}
	return out, nil
}

func (s OrderService) Advance(ctx context.Context, tx pgx.Tx, brand, id, action string, actor access.Account, in ActionInput, meta points.Metadata) (Order, error) {
	var empty Order
	if tx == nil || !validIDs(brand, id, actor.ID) || !validOrderMeta(meta, "admin") || meta.ActorID != actor.ID || ValidateActionInput(in) != nil {
		return empty, ErrInvalid
	}
	brand = strings.ToLower(brand)
	id = strings.ToLower(id)
	in.Reason = strings.TrimSpace(in.Reason)
	fresh, err := (adminsys.Store{DB: s.DB}).LockAdminAccess(ctx, tx, actor.ID, false)
	if err != nil {
		return empty, err
	}
	if fresh.SuperAdmin || !access.Authorize(fresh, "withdrawal", action, access.ScopeBrand, brand) {
		return empty, ErrDenied
	}
	if err = lockReceipt(ctx, tx, brand, in.ClientKey, meta); err != nil {
		return empty, err
	}
	hash, err := receiptHash(action, id, in)
	if err != nil {
		return empty, err
	}
	if out, ok, e := readReceipt(ctx, tx, brand, in.ClientKey, hash, meta); e != nil || ok {
		return out, e
	}
	if _, err = s.Points.LockedPolicy(ctx, tx, brand); err != nil {
		return empty, err
	}
	out, err := s.orderTx(ctx, tx, brand, id, true)
	if err != nil {
		return empty, err
	}
	if out.Version != in.Version {
		return empty, ErrVersion
	}
	out, err = s.advanceLocked(ctx, tx, out, action, in.Reason, meta)
	if err != nil {
		return empty, err
	}
	if err = saveReceipt(ctx, tx, out, in.ClientKey, hash, meta); err != nil {
		return empty, err
	}
	return out, nil
}
func (s OrderService) advanceLocked(ctx context.Context, tx pgx.Tx, out Order, action, reason string, meta points.Metadata) (Order, error) {
	next, err := TransitionTarget(out.State, action)
	if err != nil {
		return Order{}, err
	}
	if out.Version >= 9007199254740991 {
		return Order{}, points.ErrOverflow
	}
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return Order{}, err
	}
	at = at.UTC()
	from := out.State
	if next == "processing" || next == "rejected" {
		out.ReviewedAt = &at
	}
	if next == "paid" {
		paidAllocation := append([]points.Allocation(nil), out.SourceAllocation...)
		for i := range paidAllocation {
			paidAllocation[i].State = "withdrawal"
		}
		delta, e := points.AllocationDelta(paidAllocation, "withdrawal", "")
		if e != nil {
			return Order{}, e
		}
		entry, e := s.Points.Post(ctx, tx, points.Change{BrandID: out.BrandID, MemberID: out.MemberID, EntryType: "withdrawal_paid", ReferenceType: "withdrawal", ReferenceID: out.ID, OperationKey: "withdrawal-paid:" + out.ID, Reason: reason, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: paidAllocation})
		if e != nil {
			return Order{}, e
		}
		out.PaidEntryID = entry.ID
		out.CompletedAt = &at
	} else if next == "rejected" || next == "cancelled" || next == "failed" {
		original, e := s.Points.Entry(ctx, tx, out.BrandID, out.MemberID, out.ReserveEntryID)
		if e != nil {
			return Order{}, e
		}
		delta, e := points.Negate(original.Delta)
		if e != nil {
			return Order{}, e
		}
		entry, e := s.Points.Post(ctx, tx, points.Change{BrandID: out.BrandID, MemberID: out.MemberID, EntryType: "withdrawal_release", ReferenceType: "withdrawal", ReferenceID: out.ID, OperationKey: "withdrawal-release:" + out.ID, Reason: reason, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: original.Allocation, ReversalOf: original.ID})
		if e != nil {
			return Order{}, e
		}
		out.ReleaseEntryID = entry.ID
		out.CompletedAt = &at
	}
	out.State = next
	out.Version++
	out.UpdatedAt = at
	out.DecisionReason = reason
	_, err = tx.Exec(ctx, `UPDATE withdrawal_orders SET state=$3,version=$4,updated_at=$5,reviewed_at=$6,completed_at=$7,decision_reason=$8,release_entry_id=NULLIF($9,'')::uuid,paid_entry_id=NULLIF($10,'')::uuid WHERE brand_id=$1 AND id=$2`, out.BrandID, out.ID, out.State, out.Version, out.UpdatedAt, out.ReviewedAt, out.CompletedAt, reason, out.ReleaseEntryID, out.PaidEntryID)
	if err != nil {
		return Order{}, err
	}
	if next == "paid" {
		_, err = tx.Exec(ctx, `INSERT INTO withdrawal_turnover_cycles(brand_id,member_id,account_id,cutoff_at,cutoff_version,last_paid_order_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(brand_id,member_id) DO UPDATE SET cutoff_at=EXCLUDED.cutoff_at,cutoff_version=EXCLUDED.cutoff_version,last_paid_order_id=EXCLUDED.last_paid_order_id`, out.BrandID, out.MemberID, out.AccountID, out.CreatedAt, out.ReserveVersion, out.ID)
		if err != nil {
			return Order{}, err
		}
	}
	if err = orderEvidence(ctx, tx, &out, from, reason, meta); err != nil {
		return Order{}, err
	}
	return out, nil
}
