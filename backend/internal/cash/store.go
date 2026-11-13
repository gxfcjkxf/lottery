package cash

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"time"
)

type Service struct {
	Points     points.Store
	Withdrawal withdrawal.OrderService
}

func lock(ctx context.Context, tx pgx.Tx, key string) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "cash:"+key)
	return e
}
func ReadConfig(ctx context.Context, tx pgx.Tx, scope string) (Config, error) {
	out := EmptyConfig()
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT version,config FROM cash_configs WHERE scope=$1 FOR SHARE`, scope).Scan(&out.Version, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out.Rules)
	return out, e
}
func ReadBrandConfig(ctx context.Context, tx pgx.Tx, scope string) (Config, error) {
	b, err := ReadConfig(ctx, tx, scope)
	if err != nil {
		return b, err
	}
	p, err := ReadConfig(ctx, tx, "platform")
	if err != nil {
		return b, err
	}
	b.PlatformRules = p.Rules
	for i, r := range b.Rules {
		if !r.Enabled {
			b.Rules[i].Inherit = true
			continue
		}
		if r.Inherit {
			for _, pr := range p.Rules {
				if pr.Currency == r.Currency {
					b.Rules[i] = inheritLimits(r, pr)
					break
				}
			}
		}
	}
	return b, nil
}
func SaveConfig(ctx context.Context, tx pgx.Tx, scope string, in Config, meta points.Metadata) (Config, error) {
	platform := scope == "platform"
	var p Config
	if !platform {
		var err error
		p, err = ReadConfig(ctx, tx, "platform")
		if err != nil {
			return in, err
		}
	}
	for i, r := range in.Rules {
		if !r.Enabled {
			in.Rules[i] = emptyRule(r.Currency, !platform)
		} else if !platform && r.Inherit {
			pr, err := p.Rule(r.Currency)
			if err != nil {
				return in, err
			}
			in.Rules[i] = inheritLimits(r, pr)
		}
	}
	in.PlatformRules = nil
	if err := in.Validate(platform); err != nil {
		return in, err
	}

	if e := lock(ctx, tx, "config:"+scope); e != nil {
		return in, e
	}
	old, e := ReadConfig(ctx, tx, scope)
	if e != nil {
		return in, e
	}
	if old.Version != in.Version {
		return in, ErrConflict
	}
	if scope != "platform" {
		p, e := ReadConfig(ctx, tx, "platform")
		if e != nil {
			return in, e
		}
		for _, r := range in.Rules {
			if !r.Enabled {
				continue
			}
			pr, e := p.Rule(r.Currency)
			if e != nil {
				return in, e
			}
			if e = within(r.DepositMin, pr.DepositMin, pr.DepositMax, r.Currency); e != nil {
				return in, e
			}
			if e = within(r.DepositMax, pr.DepositMin, pr.DepositMax, r.Currency); e != nil {
				return in, e
			}
			if e = within(r.WithdrawalMin, pr.WithdrawalMin, pr.WithdrawalMax, r.Currency); e != nil {
				return in, e
			}
			if e = within(r.WithdrawalMax, pr.WithdrawalMin, pr.WithdrawalMax, r.Currency); e != nil {
				return in, e
			}
			fee, _ := Parse(r.Fee, r.Currency)
			maximum, _ := Parse(pr.Fee, r.Currency)
			if fee > maximum {
				return in, ErrLimit
			}
		}
	}
	raw, _ := json.Marshal(in.Rules)
	in.Version++
	_, e = tx.Exec(ctx, `INSERT INTO cash_configs(scope,version,config) VALUES($1,$2,$3) ON CONFLICT(scope) DO UPDATE SET version=EXCLUDED.version,config=EXCLUDED.config`, scope, in.Version, raw)
	if e != nil {
		return in, e
	}
	e = record(ctx, tx, scope, "config", "", old, in, meta)
	return in, e
}
func record(ctx context.Context, tx pgx.Tx, brand, action, id string, before, after any, meta points.Metadata) error {
	if brand == "platform" {
		brand = ""
	}
	_, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "cash." + action, ResourceType: "cash", ResourceID: id, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	return e
}
func Destinations(ctx context.Context, tx pgx.Tx, brand, member string) ([]Destination, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,data,enabled,version FROM cash_destinations WHERE brand_id=$1 AND member_id IS NOT DISTINCT FROM nullif($2,'')::uuid AND data->>'deleted' IS DISTINCT FROM 'true' ORDER BY id`, brand, member)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Destination{}
	for rows.Next() {
		var d Destination
		var raw []byte
		if e = rows.Scan(&d.ID, &raw, &d.Enabled, &d.Version); e != nil {
			return nil, e
		}
		var v Destination
		if e = json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		v.ID = d.ID
		v.Enabled = d.Enabled
		v.Version = d.Version
		out = append(out, v)
	}
	return out, rows.Err()
}
func SaveDestination(ctx context.Context, tx pgx.Tx, brand, member string, in Destination, meta points.Metadata) (Destination, error) {
	if in.ID == "" && in.Version != 0 || in.ID != "" && in.Version < 1 {
		return in, ErrInvalid
	}
	if e := in.Validate(); e != nil {
		return in, e
	}
	if member != "" {
		if e := memberLock(ctx, tx, brand, member); e != nil {
			return in, e
		}
	}
	if member == "" {
		if e := lock(ctx, tx, "rotation:"+brand+":"+in.Currency); e != nil {
			return in, e
		}
	}
	if member != "" {
		if !in.Enabled {
			return in, ErrInvalid
		}
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cash_destinations WHERE brand_id=$1 AND member_id=$2 AND currency=$3 AND id IS DISTINCT FROM nullif($4,'')::uuid AND data->>'deleted' IS DISTINCT FROM 'true')`, brand, member, in.Currency, in.ID).Scan(&exists); e != nil {
			return in, e
		}
		if exists {
			return in, ErrDestinationExists
		}
	}
	creating := in.ID == ""
	if creating {
		in.ID = ids.New()
		in.Version = 0
	}
	if e := lock(ctx, tx, "destination:"+in.ID); e != nil {
		return in, e
	}
	var version int64
	var oldCurrency string
	e := tx.QueryRow(ctx, `SELECT version,currency FROM cash_destinations WHERE id=$1 AND brand_id=$2 AND member_id IS NOT DISTINCT FROM nullif($3,'')::uuid AND data->>'deleted' IS DISTINCT FROM 'true' FOR UPDATE`, in.ID, brand, member).Scan(&version, &oldCurrency)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return in, e
	}
	if !creating && version == 0 || version != in.Version || version > 0 && oldCurrency != in.Currency {
		return in, ErrConflict
	}
	in.Version++
	raw, _ := json.Marshal(in)
	if version == 0 {
		_, e = tx.Exec(ctx, `INSERT INTO cash_destinations(id,brand_id,member_id,currency,data,enabled,version) VALUES($1,$2,nullif($3,'')::uuid,$4,$5,$6,1)`, in.ID, brand, member, in.Currency, raw, in.Enabled)
	} else {
		_, e = tx.Exec(ctx, `UPDATE cash_destinations SET data=$4,enabled=$5,version=$6 WHERE id=$1 AND brand_id=$2 AND member_id IS NOT DISTINCT FROM nullif($3,'')::uuid AND currency=$7`, in.ID, brand, member, raw, in.Enabled, in.Version, in.Currency)
	}
	if e != nil {
		return in, e
	}
	e = record(ctx, tx, brand, "destination.save", in.ID, nil, in, meta)
	return in, e
}

// DeleteDestination removes a member binding while retaining order references and audit history.
func DeleteDestination(ctx context.Context, tx pgx.Tx, brand, member string, id string, version int64, meta points.Metadata) (Destination, error) {
	var out Destination
	if member == "" || id == "" || version < 1 {
		return out, ErrInvalid
	}
	if e := memberLock(ctx, tx, brand, member); e != nil {
		return out, e
	}
	destinations, e := Destinations(ctx, tx, brand, member)
	if e != nil {
		return out, e
	}
	for _, d := range destinations {
		if d.ID != id {
			continue
		}
		if d.Version != version {
			return out, ErrConflict
		}
		out = d
		out.Enabled = false
		out.Version++
		raw, _ := json.Marshal(out)
		_, e = tx.Exec(ctx, `UPDATE cash_destinations SET enabled=false,version=$4,data=$5::jsonb || '{"deleted":true}'::jsonb WHERE brand_id=$1 AND member_id=$2 AND id=$3`, brand, member, id, out.Version, raw)
		if e != nil {
			return out, e
		}
		e = record(ctx, tx, brand, "destination.delete", id, d, out, meta)
		return out, e
	}
	return out, ErrConflict
}

func Balances(ctx context.Context, tx pgx.Tx, brand, member string) ([]Balance, error) {
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_owners WHERE brand_id=$1 AND id=$2)`, brand, member).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, points.ErrNotFound
	}

	rows, e := tx.Query(ctx, `SELECT c.currency,coalesce(b.deposited,0),coalesce(b.withdrawable,0),coalesce(b.reserved,0) FROM unnest(ARRAY['CNY','USD','THB','USDT']) c(currency) LEFT JOIN cash_balances b ON b.currency=c.currency AND b.brand_id=$1 AND b.member_id=$2`, brand, member)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Balance{}
	for rows.Next() {
		var b Balance
		var d, w, r int64
		if e = rows.Scan(&b.Currency, &d, &w, &r); e != nil {
			return nil, e
		}
		b.Deposited = Format(d)
		b.Withdrawable = Format(w)
		b.Reserved = Format(r)
		out = append(out, b)
	}
	return out, rows.Err()
}
func post(ctx context.Context, tx pgx.Tx, o Order, operation string, d, w, r int64) error {
	if _, e := tx.Exec(ctx, `INSERT INTO cash_balances(brand_id,member_id,currency) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, o.BrandID, o.MemberID, o.Currency); e != nil {
		return e
	}
	var before, after [3]int64
	var version int64
	if e := tx.QueryRow(ctx, `SELECT deposited,withdrawable,reserved,version FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency=$3 FOR UPDATE`, o.BrandID, o.MemberID, o.Currency).Scan(&before[0], &before[1], &before[2], &version); e != nil {
		return e
	}
	var tailRaw []byte
	var tailVersion int64
	err := tx.QueryRow(ctx, `SELECT after_snapshot,version FROM retention_cash_entries() WHERE brand_id=$1 AND member_id=$2 AND currency=$3 ORDER BY version DESC LIMIT 1`, o.BrandID, o.MemberID, o.Currency).Scan(&tailRaw, &tailVersion)
	if version == 0 {
		if !errors.Is(err, pgx.ErrNoRows) || before != [3]int64{} {
			return points.ErrCorrupt
		}
	} else {
		var tail [3]int64
		if err != nil {
			return err
		}
		if json.Unmarshal(tailRaw, &tail) != nil || tail != before || tailVersion != version {
			return points.ErrCorrupt
		}
	}
	delta := [3]int64{d, w, r}
	for i := range before {
		n := newIntSum(before[i], delta[i])
		if n == nil {
			return ErrInsufficient
		}
		after[i] = *n
	}
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	dd, _ := json.Marshal(delta)
	if _, e := tx.Exec(ctx, `INSERT INTO cash_entries(id,brand_id,member_id,currency,order_id,operation,before_snapshot,delta_snapshot,after_snapshot,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New(), o.BrandID, o.MemberID, o.Currency, o.ID, operation, b, dd, a, version+1); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE cash_balances SET deposited=$4,withdrawable=$5,reserved=$6,version=version+1 WHERE brand_id=$1 AND member_id=$2 AND currency=$3`, o.BrandID, o.MemberID, o.Currency, after[0], after[1], after[2])
	return e
}
func newIntSum(a, b int64) *int64 {
	if b < 0 && a < -b || b > 0 && a > int64(^uint64(0)>>1)-b {
		return nil
	}
	v := a + b
	return &v
}

const columns = `(SELECT CASE w.kind WHEN 'agent' THEN a.username ELSE coalesce(u.username,u.phone,u.telegram_user_id,'') END FROM wallet_owners w LEFT JOIN agent_accounts a ON a.brand_id=w.brand_id AND a.id=w.agent_account_id LEFT JOIN brand_members m ON m.brand_id=w.brand_id AND m.id=w.member_id LEFT JOIN global_users u ON u.id=m.global_user_id WHERE w.id=cash_orders.member_id AND w.brand_id=cash_orders.brand_id),id::text,brand_id::text,member_id::text,kind,currency,amount,requested_amount,points,fee,rate,state,version,destination,proof,reference,created_at,expires_at,automatic,(SELECT e.created_at FROM cash_entries e WHERE e.order_id=cash_orders.id AND e.operation='confirm') AS credited_at,sender`

func scan(row pgx.Row) (Order, error) {
	var o Order
	var a, f, requested int64
	var p points.Amount
	var raw []byte
	var at time.Time
	var credited *time.Time
	var expires *time.Time
	e := row.Scan(&o.MemberAccount, &o.ID, &o.BrandID, &o.MemberID, &o.Kind, &o.Currency, &a, &requested, &p, &f, &o.Rate, &o.State, &o.Version, &raw, &o.Proof, &o.Reference, &at, &expires, &o.Automatic, &credited, &o.Sender)
	if errors.Is(e, pgx.ErrNoRows) {
		return o, ErrConflict
	}
	if e != nil {
		return o, e
	}
	o.Amount = Format(a)
	o.RequestedAmount = Format(requested)
	o.Fee = Format(f)
	o.Points = p.String()
	o.CreatedAt = at.UTC().Format(time.RFC3339)
	if credited != nil {
		o.CreditedAt = credited.UTC().Format(time.RFC3339Nano)
	}
	if expires != nil {
		o.ExpiresAt = expires.UTC().Format(time.RFC3339)
	}
	e = json.Unmarshal(raw, &o.Destination)
	return o, e
}
func ReadOrder(ctx context.Context, tx pgx.Tx, brand, member, id string) (Order, error) {
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM cash_orders WHERE brand_id=$1 AND id=$2 AND ($3='' OR member_id=nullif($3,'')::uuid) FOR UPDATE`, brand, id, member))
}
func List(ctx context.Context, tx pgx.Tx, brand, member, kind string, limit, offset int) (map[string]any, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM cash_orders WHERE brand_id=$1 AND ($2='' OR member_id=nullif($2,'')::uuid) AND ($3='' OR kind=$3) ORDER BY cash_orders.created_at DESC,cash_orders.id DESC LIMIT $4 OFFSET $5`, brand, member, kind, limit, offset)
	if e != nil {
		return nil, e
	}
	out := []Order{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var count string
	e = tx.QueryRow(ctx, `SELECT count(*)::text FROM cash_orders WHERE brand_id=$1 AND ($2='' OR member_id=nullif($2,'')::uuid) AND ($3='' OR kind=$3)`, brand, member, kind).Scan(&count)
	return map[string]any{"items": out, "total_count": count}, e
}
func insert(ctx context.Context, tx pgx.Tx, o Order, expires bool) error {
	a, e := Parse(o.Amount, o.Currency)
	if e != nil {
		return e
	}
	f, e := Parse(o.Fee, o.Currency)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(o.Destination)
	_, e = tx.Exec(ctx, `INSERT INTO cash_orders(id,brand_id,member_id,kind,currency,amount,points,fee,rate,state,destination_id,destination,expires_at,requested_amount,automatic) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,nullif($11,'')::uuid,$12,CASE WHEN $13 THEN now()+interval '30 minutes' ELSE NULL END,$6,$14)`, o.ID, o.BrandID, o.MemberID, o.Kind, o.Currency, a, o.Points, f, o.Rate, o.State, o.Destination.ID, raw, expires, o.Automatic)
	return e
}
func within(value, min, max, currency string) error {
	n, e := Parse(value, currency)
	if e != nil {
		return e
	}
	a, e := Parse(min, currency)
	if e != nil {
		return e
	}
	b, e := Parse(max, currency)
	if e != nil {
		return e
	}
	if n < a || n > b {
		return ErrLimit
	}
	return nil
}
func rules(ctx context.Context, tx pgx.Tx, brand, currency string) (Rule, Rule, error) {
	p, e := ReadConfig(ctx, tx, "platform")
	if e != nil {
		return Rule{}, Rule{}, e
	}
	b, e := ReadBrandConfig(ctx, tx, brand)
	if e != nil {
		return Rule{}, Rule{}, e
	}
	pr, e := p.Rule(currency)
	if e != nil {
		return Rule{}, Rule{}, e
	}
	br, e := b.Rule(currency)
	return pr, br, e
}
func memberLock(ctx context.Context, tx pgx.Tx, brand, member string) error {
	return lock(ctx, tx, "member:"+brand+":"+member)
}
func checkMember(ctx context.Context, tx pgx.Tx, brand, member string, deposit bool) error {
	var status, m, u string
	e := tx.QueryRow(ctx, `SELECT b.status,m.status,u.status FROM brands b JOIN brand_members m ON m.brand_id=b.id JOIN global_users u ON u.id=m.global_user_id WHERE b.id=$1 AND m.id=$2 FOR SHARE OF b,m,u`, brand, member).Scan(&status, &m, &u)
	if e != nil {
		return e
	}
	if status == "disabled" || deposit && status != "active" || m != "normal" || u != "active" {
		return ErrUnavailable
	}
	return nil
}
func checkOwner(ctx context.Context, tx pgx.Tx, brand, owner string, meta points.Metadata) error {
	if meta.ActorType != "agent" {
		return checkMember(ctx, tx, brand, owner, false)
	}
	if meta.ActorID != owner {
		return ErrInvalid
	}
	var active bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_accounts a JOIN brands b ON b.id=a.brand_id JOIN agent_nodes n ON n.brand_id=a.brand_id AND n.member_id=a.id WHERE a.brand_id=$1 AND a.id=$2 AND b.status='active' AND NOT EXISTS(SELECT 1 FROM agent_nodes p WHERE p.brand_id=n.brand_id AND p.id=ANY(n.path) AND p.config->>'status'<>'active'))`, brand, owner).Scan(&active)
	if e != nil {
		return e
	}
	if !active {
		return ErrUnavailable
	}
	return nil
}

func (s Service) Exchange(ctx context.Context, tx pgx.Tx, brand, member string, in Input, meta points.Metadata) (Order, error) {
	o := Order{ID: ids.New(), BrandID: brand, MemberID: member, Currency: in.Currency, Fee: "0", Rate: in.Rate, State: "completed", Version: 1}
	pr, _, e := rules(ctx, tx, brand, in.Currency)
	if e != nil {
		return o, e
	}
	if pr.Rate != in.Rate {
		return o, ErrRate
	}
	if e = checkOwner(ctx, tx, brand, member, meta); e != nil {
		return o, e
	}
	// Match existing manual recharge lock order: quota before point wallet.
	var quota points.Amount
	if in.Direction == "to_points" {
		e = tx.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1 FOR UPDATE`, brand).Scan(&quota)
		if errors.Is(e, pgx.ErrNoRows) {
			return o, finance.ErrRechargeQuota
		}
		if e != nil {
			return o, e
		}
	}
	if e = memberLock(ctx, tx, brand, member); e != nil {
		return o, e
	}
	if in.Direction == "to_points" {
		n, e := Parse(in.Amount, in.Currency)
		if e != nil || n <= 0 {
			return o, ErrInvalid
		}
		v, e := Convert(n, pr.Rate, true, in.Currency)
		if e != nil {
			return o, e
		}
		p := points.Amount(v)
		policy, err := s.Points.LockedPolicy(ctx, tx, brand)
		if err != nil {
			return o, err
		}
		if err = policy.CheckRecharge(p); err != nil {
			return o, err
		}
		if quota < p {
			return o, finance.ErrRechargeQuota
		}
		o.Kind = "to_points"
		o.Amount = Format(n)
		o.Points = p.String()
		if e = insert(ctx, tx, o, false); e != nil {
			return o, e
		}
		// Prefer unconverted recharge funds; re-exchanged withdrawable funds also
		// become recharge points and must satisfy turnover before leaving again.
		balances, e := Balances(ctx, tx, brand, member)
		if e != nil {
			return o, e
		}
		var d, w int64
		for _, b := range balances {
			if b.Currency == in.Currency {
				d, _ = Parse(b.Deposited, in.Currency)
				w, _ = Parse(b.Withdrawable, in.Currency)
			}
		}
		use := n
		if use > d {
			use = d
		}
		if n-use > w {
			return o, ErrInsufficient
		}
		if e = post(ctx, tx, o, "exchange", -use, -(n - use), 0); e != nil {
			return o, e
		}
		var delta points.Balance
		delta[0][0] = p
		entry, e := s.Points.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "currency_exchange_credit", ReferenceType: "cash_order", ReferenceID: o.ID, OperationKey: "cash-exchange:" + o.ID, ActorType: meta.ActorType, ActorID: meta.ActorID, RequestID: meta.RequestID, IP: meta.IP, Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: p}}})
		if e != nil {
			return o, e
		}
		if _, e = tx.Exec(ctx, `UPDATE brand_recharge_quotas SET available=available-$2 WHERE brand_id=$1`, brand, p); e != nil {
			return o, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO cash_quota_entries(order_id,brand_id,member_id,points,before_available,after_available) VALUES($1,$2,$3,$4,$5,$6)`, o.ID, brand, member, p, quota, quota-p); e != nil {
			return o, e
		}
		_, e = tx.Exec(ctx, `UPDATE cash_orders SET point_entry_id=$2 WHERE id=$1`, o.ID, entry.ID)
		if e != nil {
			return o, e
		}
	} else if in.Direction == "from_points" {
		p, e := points.ParseAmount(in.Amount)
		if e != nil || p <= 0 {
			return o, ErrInvalid
		}
		n, e := Convert(int64(p), pr.Rate, false, in.Currency)
		if e != nil {
			return o, e
		}
		o.Kind = "from_points"
		o.Amount = Format(n)
		o.Points = p.String()
		if e = insert(ctx, tx, o, false); e != nil {
			return o, e
		}
		var entry points.Entry
		var evidence json.RawMessage
		if meta.ActorType == "agent" {
			entry, evidence, e = s.convertAgent(ctx, tx, brand, member, o.ID, p, meta)
		} else {
			entry, evidence, e = s.Withdrawal.ConvertTx(ctx, tx, brand, member, o.ID, p, meta)
		}
		if e != nil {
			return o, e
		}
		if e = post(ctx, tx, o, "exchange", 0, n, 0); e != nil {
			return o, e
		}
		if _, e = tx.Exec(ctx, `UPDATE cash_orders SET point_entry_id=$2,evidence=$3 WHERE id=$1`, o.ID, entry.ID, []byte(evidence)); e != nil {
			return o, e
		}
	} else {
		return o, ErrInvalid
	}
	if e = record(ctx, tx, brand, "exchange", o.ID, nil, o, meta); e != nil {
		return o, e
	}
	return ReadOrder(ctx, tx, brand, member, o.ID)
}
