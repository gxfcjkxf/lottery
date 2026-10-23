package betting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestConfiguredConcurrentRetriesUseSingleDebit(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	fundBettingWallet(t, c.f, 100)
	type response struct {
		data   []byte
		status int
		err    error
	}
	results := make(chan response, 6)
	for i := 0; i < 6; i++ {
		go func() {
			r, err := configuredPlace(t, c, "same-concurrent-purchase", c.input)
			results <- response{r.Data, r.Status, err}
		}()
	}
	var original []byte
	for i := 0; i < 6; i++ {
		r := <-results
		if r.err != nil || r.status != 201 {
			t.Fatal("concurrent retry", r.status, r.err)
		}
		if i == 0 {
			original = r.data
		} else if !bytes.Equal(original, r.data) {
			t.Fatal("concurrent retries returned different purchases")
		}
	}
	wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if err != nil || wallet.AvailablePoints != 90 {
		t.Fatal("concurrent retry balance", wallet, err)
	}
	var orders, debits int
	if err = c.f.db.QueryRow(ctx, `SELECT count(*),count(DISTINCT debit_entry_id) FROM configured_orders WHERE period_id=$1`, c.p.ID).Scan(&orders, &debits); err != nil || orders != 1 || debits != 1 {
		t.Fatal("concurrent retry evidence", orders, debits, err)
	}
	t.Log("six concurrent requests with the same idempotency key: one purchase, one debit, balance 100 -> 90")
}

func TestConfiguredFundingGuardRecoveryRetriesFailedSettlementOnce(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	fundBettingWallet(t, c.f, 100)
	first, err := configuredPlace(t, c, "migration-retry", c.input)
	if err != nil {
		t.Fatal(err)
	}
	var order ConfiguredOrder
	if err = json.Unmarshal(first.Data, &order); err != nil {
		t.Fatal(err)
	}

	// Inject the reproduced alias fault into the installed guard in this owned schema.
	var currentGuard string
	if err = c.f.db.QueryRow(ctx, `SELECT pg_get_functiondef('guard_configured_order()'::regprocedure)`).Scan(&currentGuard); err != nil {
		t.Fatal(err)
	}
	brokenGuard := strings.NewReplacer("credit", "l", "settlement_ledger", "l").Replace(currentGuard)
	if brokenGuard == currentGuard {
		t.Fatal("funding guard fault was not injected")
	}
	if _, err = c.f.db.Exec(ctx, brokenGuard); err != nil {
		t.Fatal(err)
	}
	configuredPublishDraw(t, c, 7)
	n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10)
	var sqlError *pgconn.PgError
	if n != 0 || !errors.As(err, &sqlError) || sqlError.Code != "42702" {
		t.Fatal("expected original ambiguous-column failure", n, err)
	}
	assertEvidence := func(balance points.Amount, status string, prizes int) {
		t.Helper()
		wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
		if err != nil || wallet.AvailablePoints != balance {
			t.Fatal("wallet evidence", wallet, err)
		}
		var actualStatus string
		var debits, credits int
		if err = c.f.db.QueryRow(ctx, `SELECT status FROM configured_orders WHERE id=$1`, order.ID).Scan(&actualStatus); err != nil || actualStatus != status {
			t.Fatal("order status", actualStatus, err)
		}
		if err = c.f.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE entry_type='bet'),count(*) FILTER(WHERE entry_type='prize') FROM point_ledger_entries WHERE reference_type='configured_order' AND reference_id=$1`, order.ID).Scan(&debits, &credits); err != nil || debits != 1 || credits != prizes {
			t.Fatal("ledger evidence", debits, credits, err)
		}
	}
	assertEvidence(90, "placed", 0)
	if _, err = c.f.db.Exec(ctx, currentGuard); err != nil {
		t.Fatal(err)
	}
	if n, err = c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 1 {
		t.Fatal("retry after guard recovery", n, err)
	}
	if n, err = c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 {
		t.Fatal("repeat after recovery", n, err)
	}
	if replay, err := configuredPlace(t, c, "migration-retry", c.input); err != nil || string(replay.Data) != string(first.Data) {
		t.Fatal("purchase replay after recovery", err)
	}
	assertEvidence(115, "settled", 1)
	t.Log("SQL failure rolls back payout; restoring the installed guard retries the original order once: balance 90 -> 115, debit=1, prize=1")
}
