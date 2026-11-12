package cash

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
)

// Rotation is serialized per brand/currency. A payment order pins the account,
// details and allocation epoch; browser refreshes never allocate again.
func allocate(ctx context.Context, tx pgx.Tx, brand string, r Rule, amount int64) (Destination, int64, error) {
	if e := lock(ctx, tx, "rotation:"+brand+":"+r.Currency); e != nil {
		return Destination{}, 0, e
	}
	if e := expire(ctx, tx, brand, r.Currency); e != nil {
		return Destination{}, 0, e
	}
	var current string
	e := tx.QueryRow(ctx, `SELECT destination_id::text FROM cash_rotation WHERE brand_id=$1 AND currency=$2`, brand, r.Currency).Scan(&current)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return Destination{}, 0, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,data,version,assigned_count,assigned_amount,rotation_epoch FROM cash_destinations WHERE brand_id=$1 AND member_id IS NULL AND currency=$2 AND enabled ORDER BY id FOR UPDATE`, brand, r.Currency)
	if e != nil {
		return Destination{}, 0, e
	}
	type candidate struct {
		d                    Destination
		count, amount, epoch int64
	}
	list := []candidate{}
	for rows.Next() {
		var c candidate
		var raw []byte
		var id string
		var version int64
		if e = rows.Scan(&id, &raw, &version, &c.count, &c.amount, &c.epoch); e != nil {
			rows.Close()
			return Destination{}, 0, e
		}
		if e = json.Unmarshal(raw, &c.d); e != nil {
			rows.Close()
			return Destination{}, 0, e
		}
		c.d.ID = id
		c.d.Version = version
		c.d.Enabled = true
		list = append(list, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return Destination{}, 0, e
	}
	if len(list) == 0 {
		return Destination{}, 0, ErrUnavailable
	}
	start := 0
	for i, c := range list {
		if c.d.ID == current {
			start = i
			break
		}
	}
	countThreshold, _ := strconv.ParseInt(r.Threshold, 10, 64)
	amountThreshold, _ := Parse(r.Threshold, r.Currency)
	for offset := 0; offset < len(list)+1; offset++ {
		i := (start + offset) % len(list)
		c := list[i]
		reset := offset > 0 || c.d.ID != current
		if reset {
			c.count = 0
			c.amount = 0
			c.epoch++
		}
		if r.Mode == "count" && c.count >= countThreshold || r.Mode == "amount" && (amount > amountThreshold || c.amount > amountThreshold-amount) {
			continue
		}
		_, e = tx.Exec(ctx, `UPDATE cash_destinations SET assigned_count=$2,assigned_amount=$3,rotation_epoch=$4 WHERE id=$1`, c.d.ID, c.count+1, c.amount+amount, c.epoch)
		if e != nil {
			return Destination{}, 0, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO cash_rotation(brand_id,currency,destination_id) VALUES($1,$2,$3) ON CONFLICT(brand_id,currency) DO UPDATE SET destination_id=EXCLUDED.destination_id`, brand, r.Currency, c.d.ID)
		return c.d, c.epoch, e
	}
	return Destination{}, 0, ErrLimit
}
func expire(ctx context.Context, tx pgx.Tx, brand, currency string) error {
	// An unsubmitted order expires after 30 minutes. Submitted payments retain
	// their allocation until reviewed; late payments remain manually confirmable.
	_, e := tx.Exec(ctx, `WITH expired AS (UPDATE cash_orders SET state='expired',version=version+1 WHERE brand_id=$1 AND currency=$2 AND kind='deposit' AND state='pending' AND expires_at<=now() RETURNING destination_id,rotation_epoch,amount), released AS (SELECT destination_id,rotation_epoch,count(*) n,sum(amount) amount FROM expired GROUP BY destination_id,rotation_epoch) UPDATE cash_destinations d SET assigned_count=d.assigned_count-r.n,assigned_amount=d.assigned_amount-r.amount FROM released r WHERE d.id=r.destination_id AND d.rotation_epoch=r.rotation_epoch`, brand, currency)
	return e
}
func releaseAllocation(ctx context.Context, tx pgx.Tx, o Order) error {
	_, e := tx.Exec(ctx, `UPDATE cash_destinations d SET assigned_count=d.assigned_count-1,assigned_amount=d.assigned_amount-o.amount FROM cash_orders o WHERE o.id=$1 AND d.id=o.destination_id AND d.rotation_epoch=o.rotation_epoch`, o.ID)
	return e
}
func (s Service) Deposit(ctx context.Context, tx pgx.Tx, brand, member string, in Input, meta points.Metadata) (Order, error) {
	o := Order{ID: ids.New(), BrandID: brand, MemberID: member, Kind: "deposit", Currency: in.Currency, Amount: in.Amount, Points: "0", Fee: "0", State: "pending", Version: 1}
	if e := checkMember(ctx, tx, brand, member, true); e != nil {
		return o, e
	}
	return s.deposit(ctx, tx, brand, member, in, meta)
}

// DepositAgent uses the independent agent account, never a member session.
func (s Service) DepositAgent(ctx context.Context, tx pgx.Tx, brand, account string, in Input, meta points.Metadata) (Order, error) {
	var active bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_accounts a JOIN brands b ON b.id=a.brand_id JOIN agent_nodes n ON n.brand_id=a.brand_id AND n.member_id=a.id WHERE a.brand_id=$1 AND a.id=$2 AND b.status='active' AND NOT EXISTS(SELECT 1 FROM agent_nodes p WHERE p.brand_id=n.brand_id AND p.id=ANY(n.path) AND p.config->>'status'<>'active'))`, brand, account).Scan(&active)
	if e != nil {
		return Order{}, e
	}
	if !active {
		return Order{}, ErrUnavailable
	}
	return s.deposit(ctx, tx, brand, account, in, meta)
}

func (s Service) deposit(ctx context.Context, tx pgx.Tx, brand, member string, in Input, meta points.Metadata) (Order, error) {
	o := Order{ID: ids.New(), BrandID: brand, MemberID: member, Kind: "deposit", Currency: in.Currency, Amount: in.Amount, Points: "0", Fee: "0", State: "pending", Version: 1}
	p, b, e := rules(ctx, tx, brand, in.Currency)
	if e != nil {
		return o, e
	}
	if e = within(in.Amount, p.DepositMin, p.DepositMax, in.Currency); e != nil {
		return o, e
	}
	if e = within(in.Amount, b.DepositMin, b.DepositMax, in.Currency); e != nil {
		return o, e
	}
	n, e := Parse(in.Amount, in.Currency)
	if e != nil || n <= 0 {
		return o, ErrInvalid
	}
	if e = memberLock(ctx, tx, brand, member); e != nil {
		return o, e
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cash_orders WHERE brand_id=$1 AND member_id=$2 AND kind='deposit' AND currency=$3 AND (state='submitted' OR state='pending' AND expires_at>now()))`, brand, member, in.Currency).Scan(&active); e != nil {
		return o, e
	}
	if active {
		return o, ErrDepositPending
	}
	var d Destination
	var epoch int64
	d, epoch, e = allocate(ctx, tx, brand, b, n)
	if e != nil {
		return o, e
	}
	o.Destination = d
	if in.Currency == "USDT" {
		fee, err := reserveUniqueAmount(ctx, tx, d.Number, n)
		if err != nil {
			return o, err
		}
		o.Fee = Format(fee)
		o.Automatic = true
	}
	if e = insert(ctx, tx, o, true); e != nil {
		return o, e
	}
	if o.Automatic {
		fee, _ := Parse(o.Fee, o.Currency)
		if _, e = tx.Exec(ctx, `INSERT INTO cash_usdt_amounts(address,payable,order_id) VALUES($1,$2,$3)`, d.Number, n+fee, o.ID); e != nil {
			return o, e
		}
	}
	if epoch > 0 {
		if _, e = tx.Exec(ctx, `UPDATE cash_orders SET rotation_epoch=$2 WHERE id=$1`, o.ID, epoch); e != nil {
			return o, e
		}
	}
	if e = record(ctx, tx, brand, "deposit.create", o.ID, nil, o, meta); e != nil {
		return o, e
	}
	return ReadOrder(ctx, tx, brand, member, o.ID)
}
func (s Service) Withdraw(ctx context.Context, tx pgx.Tx, brand, member string, in Input, meta points.Metadata) (Order, error) {
	o := Order{ID: ids.New(), BrandID: brand, MemberID: member, Kind: "withdrawal", Currency: in.Currency, Amount: in.Amount, Points: "0", State: "reviewing", Version: 1}
	if e := checkOwner(ctx, tx, brand, member, meta); e != nil {
		return o, e
	}
	p, b, e := rules(ctx, tx, brand, in.Currency)
	if e != nil {
		return o, e
	}
	if e = within(in.Amount, p.WithdrawalMin, p.WithdrawalMax, in.Currency); e != nil {
		return o, e
	}
	if e = within(in.Amount, b.WithdrawalMin, b.WithdrawalMax, in.Currency); e != nil {
		return o, e
	}
	n, e := Parse(in.Amount, in.Currency)
	if e != nil || n <= 0 {
		return o, ErrInvalid
	}
	f, e := Parse(b.Fee, in.Currency)
	if e != nil || f >= n {
		return o, ErrInvalid
	}
	maxFee, _ := Parse(p.Fee, in.Currency)
	if f > maxFee {
		return o, ErrLimit
	}
	expectedFee, feeErr := Parse(in.Fee, in.Currency)
	if feeErr != nil {
		return o, ErrInvalid
	}
	if expectedFee != f {
		return o, ErrConflict
	}
	o.Fee = b.Fee
	if e = memberLock(ctx, tx, brand, member); e != nil {
		return o, e
	}
	destinations, e := Destinations(ctx, tx, brand, member)
	if e != nil {
		return o, e
	}
	found := false
	for _, d := range destinations {
		if d.ID == in.DestinationID && d.Enabled && d.Currency == in.Currency && d.Validate() == nil {
			if d.Version != in.DestinationVersion {
				return o, ErrConflict
			}
			o.Destination = d
			found = true
		}
	}
	if !found {
		return o, ErrAccount
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cash_orders WHERE brand_id=$1 AND member_id=$2 AND kind='withdrawal' AND state IN('reviewing','processing'))`, brand, member).Scan(&active); e != nil {
		return o, e
	}
	if active {
		return o, ErrConflict
	}
	if e = insert(ctx, tx, o, false); e != nil {
		return o, e
	}
	if e = post(ctx, tx, o, "reserve", 0, -n, n); e != nil {
		return o, e
	}
	if e = record(ctx, tx, brand, "withdrawal.create", o.ID, nil, o, meta); e != nil {
		return o, e
	}
	return ReadOrder(ctx, tx, brand, member, o.ID)
}
func hashReference(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func (s Service) Advance(ctx context.Context, tx pgx.Tx, brand, member, id, action string, in Input, meta points.Metadata) (Order, error) {
	// Discover immutable scope before taking member -> rotation -> order locks.
	var owner, currency, kind string
	e := tx.QueryRow(ctx, `SELECT member_id::text,currency,kind FROM cash_orders WHERE brand_id=$1 AND id=$2 AND ($3='' OR member_id=nullif($3,'')::uuid)`, brand, id, member).Scan(&owner, &currency, &kind)
	if errors.Is(e, pgx.ErrNoRows) {
		return Order{}, ErrConflict
	}
	if e != nil {
		return Order{}, e
	}
	if e = memberLock(ctx, tx, brand, owner); e != nil {
		return Order{}, e
	}
	if kind == "deposit" {
		if e = lock(ctx, tx, "rotation:"+brand+":"+currency); e != nil {
			return Order{}, e
		}
	}
	if currency == "USDT" && kind == "deposit" && action == "confirm" {
		reference := strings.ToLower(in.Reference)
		if e = lock(ctx, tx, "usdt-transaction:"+reference); e != nil {
			return Order{}, e
		}
		var used bool
		// Manual review consumes the entire hash; automatic review consumes its log.
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cash_orders prior JOIN cash_orders target ON target.id=$2 WHERE prior.currency='USDT' AND prior.kind='deposit' AND prior.state='confirmed' AND prior.reference=$1 AND (prior.event_index=-1 OR target.event_index=-1 OR prior.event_index=target.event_index))`, reference, id).Scan(&used)
		if e != nil {
			return Order{}, e
		}
		if used {
			return Order{}, ErrConflict
		}
	}
	o, e := ReadOrder(ctx, tx, brand, member, id)
	if e != nil {
		return o, e
	}
	if o.Version != in.Version {
		return o, ErrConflict
	}
	before := o
	n, _ := Parse(o.Amount, o.Currency)
	if meta.ActorType == "user" || meta.ActorType == "agent" {
		if action == "submit" && o.Kind == "deposit" && (o.State == "pending" || o.State == "expired") {
			if o.Currency != "USDT" && strings.TrimSpace(in.Proof) == "" && in.Reference == "" || len(in.Proof) > 500 || len(in.Reference) > 200 {
				return o, ErrInvalid
			}
			if o.Currency == "USDT" && in.Reference != "" && !hashReference(strings.ToLower(in.Reference)) {
				return o, ErrInvalid
			}
			o.Proof = in.Proof
			o.Reference = in.Reference
			if o.Currency == "USDT" {
				o.Reference = strings.ToLower(in.Reference)
			}
			o.State = "submitted"
			if before.State == "expired" {
				if _, e = tx.Exec(ctx, `UPDATE cash_orders SET rotation_epoch=NULL WHERE id=$1`, o.ID); e != nil {
					return o, e
				}
			}
		} else if action == "cancel" && (o.Kind == "deposit" && o.State == "pending" || o.Kind == "withdrawal" && o.State == "reviewing") {
			if o.Kind == "deposit" {
				e = releaseAllocation(ctx, tx, o)
			} else {
				e = post(ctx, tx, o, "release", 0, n, -n)
			}
			o.State = "cancelled"
		} else {
			return o, ErrConflict
		}
	} else if meta.ActorType == "admin" || meta.ActorType == "system" && action == "confirm" && o.Kind == "deposit" {
		switch {
		case action == "confirm" && o.Kind == "deposit" && (o.State == "pending" || o.State == "submitted" || o.State == "expired"):
			actual, e := Parse(in.Amount, o.Currency)
			if e != nil || actual <= 0 {
				return o, ErrInvalid
			}
			if len(in.Reference) > 200 || strings.TrimSpace(in.Reference) == "" {
				return o, ErrInvalid
			}
			if o.Currency == "USDT" && !hashReference(strings.ToLower(in.Reference)) {
				return o, ErrInvalid
			}
			if o.Automatic {
				fee, _ := Parse(o.Fee, o.Currency)
				if actual <= fee {
					return o, ErrInvalid
				}
				actual -= fee
			}
			o.Amount = Format(actual)
			o.Reference = in.Reference
			if o.Currency == "USDT" {
				o.Reference = strings.ToLower(in.Reference)
			}
			o.State = "confirmed"
			// Account capacity follows the actual confirmed amount; excess payment is
			// recorded truthfully even when it exceeds the configured allocation cap.
			if _, e = tx.Exec(ctx, `UPDATE cash_destinations d SET assigned_amount=d.assigned_amount+$2-o.amount FROM cash_orders o WHERE o.id=$1 AND d.id=o.destination_id AND d.rotation_epoch=o.rotation_epoch AND o.state<>'expired'`, o.ID, actual); e != nil {
				return o, e
			}
			if before.State == "expired" {
				if _, e = tx.Exec(ctx, `UPDATE cash_orders SET rotation_epoch=NULL WHERE id=$1`, o.ID); e != nil {
					return o, e
				}
			}
			e = post(ctx, tx, o, "confirm", actual, 0, 0)
		case action == "reject" && o.Kind == "deposit" && (o.State == "pending" || o.State == "submitted"):
			e = releaseAllocation(ctx, tx, o)
			o.State = "rejected"
		case action == "approve" && o.Kind == "withdrawal" && o.State == "reviewing":
			o.State = "processing"
		case action == "reject" && o.Kind == "withdrawal" && o.State == "reviewing":
			e = post(ctx, tx, o, "release", 0, n, -n)
			o.State = "rejected"
		case action == "fail" && o.Kind == "withdrawal" && o.State == "processing":
			if strings.TrimSpace(in.Proof) == "" || len(in.Proof) > 500 {
				return o, ErrInvalid
			}
			o.Proof = in.Proof
			e = post(ctx, tx, o, "release", 0, n, -n)
			o.State = "failed"
		case action == "paid" && o.Kind == "withdrawal" && o.State == "processing":
			if len(in.Reference) > 200 || strings.TrimSpace(in.Reference) == "" {
				return o, ErrInvalid
			}
			if o.Currency == "USDT" && !hashReference(strings.ToLower(in.Reference)) {
				return o, ErrInvalid
			}
			o.Reference = in.Reference
			if o.Currency == "USDT" {
				o.Reference = strings.ToLower(in.Reference)
			}
			e = post(ctx, tx, o, "paid", 0, 0, -n)
			o.State = "paid"
		default:
			return o, ErrConflict
		}
	} else {
		return o, ErrInvalid
	}
	if e != nil {
		return o, e
	}
	o.Version++
	a, _ := Parse(o.Amount, o.Currency)
	_, e = tx.Exec(ctx, `UPDATE cash_orders SET state=$3,version=$4,amount=$5,proof=$6,reference=$7 WHERE brand_id=$1 AND id=$2`, brand, o.ID, o.State, o.Version, a, o.Proof, o.Reference)
	if e != nil {
		return o, e
	}
	if e = record(ctx, tx, brand, "order."+action, o.ID, before, o, meta); e != nil {
		return o, e
	}
	return ReadOrder(ctx, tx, brand, member, o.ID)
}
