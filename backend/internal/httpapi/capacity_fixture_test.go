//go:build capacity && !windows

package httpapi

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const capacityFunding points.Amount = 100000

type capacityUser struct {
	Brand, Member, Token string
	Body                 []byte
}
type capacityFixture struct {
	DB       *pgxpool.Pool
	Server   *httptest.Server
	Users    []capacityUser
	HTTP     *http.Client
	PoolSize int32
}

func capacitySafety(t *testing.T) {
	t.Helper()
	if os.Getenv("LOTTERY_CAPACITY_CONFIRM_ISOLATED") != "yes" {
		t.Fatal("capacity requires LOTTERY_CAPACITY_CONFIRM_ISOLATED=yes and an isolated loopback PostgreSQL")
	}
	u, e := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if e != nil || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "55432" {
		t.Fatal("capacity must use a dedicated loopback database port, not the original development database")
	}
}
func capacityTx(t *testing.T, p *pgxpool.Pool, run func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = run(tx); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func capacityRule(t *testing.T, p *pgxpool.Pool, brand, code string, window, drawDelay time.Duration) betting.Input {
	t.Helper()
	ctx := context.Background()
	rs := rulebook.Store{DB: p}
	creatorID, reviewerID := ids.New(), ids.New()
	for _, id := range []string{creatorID, reviewerID} {
		if _, e := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'capacity-fixture-only-no-login')`, id, "capacity_"+strings.ReplaceAll(id, "-", "")); e != nil {
			t.Fatal(e)
		}
	}
	actor := func(id string, actions ...string) access.Account {
		r := access.Role{BrandID: brand, Permissions: []access.Permission{{Resource: "game", Action: "write", Scope: access.ScopeBrand}}}
		for _, action := range actions {
			r.Permissions = append(r.Permissions, access.Permission{Resource: "rule", Action: action, Scope: access.ScopeBrand})
		}
		return access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{r}}
	}
	creator, reviewer := actor(creatorID, "write", "validate", "submit"), actor(reviewerID, "review")
	three := 3
	definition := rules.Definition{SchemaVersion: 1, Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1, PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: &three}, Odds: "10", Exclusive: true}}, Rounding: "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	meta := points.Metadata{ActorType: "admin", ActorID: creatorID, RequestID: ids.New()}
	var game rulebook.Game
	var play rulebook.Play
	var version rulebook.Version
	var period rulebook.Period
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		game, e = rs.CreateGame(ctx, tx, brand, creator, code, "Capacity fixture", definition.Model, "UTC", "capacity isolated fixture", meta)
		return e
	})
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		play, e = rs.CreatePlay(ctx, tx, brand, creator, game.ID, "exact", "Exact", "capacity isolated fixture", meta)
		return e
	})
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		version, e = rs.CreateVersion(ctx, tx, brand, creator, play.ID, definition, "immediate", "capacity isolated fixture", meta)
		return e
	})
	stake, prize := points.Amount(1), points.Amount(10)
	won := true
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		version, e = rs.Validate(ctx, tx, brand, creator, version.ID, version.Version, []rules.ValidationCase{{Name: "exact", Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1, ExpectedBetPoints: &stake, ExpectedPrizePoints: &prize, ExpectedWon: &won}}, "capacity isolated validation", meta)
		return e
	})
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		version, e = rs.Submit(ctx, tx, brand, creator, version.ID, version.Version, "capacity isolated submission", meta)
		return e
	})
	meta.ActorID = reviewerID
	meta.RequestID = ids.New()
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		version, e = rs.Review(ctx, tx, brand, reviewer, version.ID, version.Version, true, true, "capacity independent review", meta)
		return e
	})
	now := time.Now().UTC()
	capacityTx(t, p, func(tx pgx.Tx) error {
		var e error
		period, e = rs.OpenPeriod(ctx, tx, brand, game.ID, "capacity-1", now.Add(-time.Minute), now.Add(window), now.Add(drawDelay))
		return e
	})
	svc := betting.Service{DB: p}
	bp, e := svc.BrandPolicy(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	gp, e := svc.GamePolicy(ctx, brand, game.ID)
	if e != nil {
		t.Fatal(e)
	}
	return betting.Input{PeriodID: period.ID, PlayID: play.ID, RuleVersionID: version.ID, Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Multiplier: 1, PolicyVersions: &betting.PolicyVersions{Brand: bp.Version, Game: gp.Version}}
}
func newCapacityFixture(t *testing.T, userCount, brands int, poolSize int32) capacityFixture {
	return newCapacityMembers(t, userCount, brands, poolSize, false)
}

// Commission members join using a real agent code at first membership creation.
// Existing immutable member attribution is never changed by a capacity test.
func newCapacityMembers(t *testing.T, userCount, brands int, poolSize int32, agentJoins bool) capacityFixture {
	t.Helper()
	capacitySafety(t)
	if userCount < 1 || userCount > 5000 || brands < 1 || brands > 2 || poolSize < 4 || poolSize > 100 {
		t.Fatal("capacity fixture limits invalid")
	}
	base := testdb.New(t)
	cfg := base.Config()
	cfg.MaxConns = poolSize
	p, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	users, e := identity.New(p)
	if e != nil {
		t.Fatal(e)
	}
	key := make([]byte, 32)
	engine, e := mutation.New(p, key)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(New(Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: p}, WithdrawalEligibility: withdrawal.TurnoverChecker{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	t.Cleanup(server.Close)
	transport := &http.Transport{MaxIdleConns: 1000, MaxIdleConnsPerHost: 1000, MaxConnsPerHost: 1000}
	t.Cleanup(transport.CloseIdleConnections)
	f := capacityFixture{DB: p, Server: server, HTTP: &http.Client{Transport: transport, Timeout: 12 * time.Second}, PoolSize: poolSize}
	brandIDs := []string{"0199a000-0000-7000-8000-000000000001", "0199a000-0000-7000-8000-000000000002"}
	codes := []string{"aurora", "harbor"}
	inputs := make([]betting.Input, brands)
	window := time.Hour
	if cutoff := capacityInt(t, "LOTTERY_CAPACITY_CUTOFF_SECONDS", 0, 0, 300); cutoff > 0 {
		window = time.Duration(cutoff) * time.Second
	}
	for i := range inputs {
		inputs[i] = capacityRule(t, p, brandIDs[i], "capacity_game", window, 2*time.Hour)
	}
	ctx := context.Background()
	agentCodes := make([]string, brands)
	for i := 0; i < userCount; i++ {
		b := i % brands
		joinMethod := "domain"
		selected := json.RawMessage(`{}`)
		if agentJoins && i >= brands {
			joinMethod = "agent_code"
			selected, e = json.Marshal(map[string]string{"kind": "agent", "code": agentCodes[b]})
			if e != nil {
				t.Fatal(e)
			}
		}
		user, member, account := ids.New(), ids.New(), ids.New()
		token, e := authcrypto.NewSessionToken()
		if e != nil {
			t.Fatal(e)
		}
		digest := authcrypto.DigestSessionToken(token)
		capacityTx(t, p, func(tx pgx.Tx) error {
			for _, q := range []struct {
				SQL  string
				Args []any
			}{
				{`INSERT INTO global_users(id,username)VALUES($1,$2)`, []any{user, "capacity_" + strings.ReplaceAll(user, "-", "")}},
				{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,terms_accepted,privacy_policy_version,service_terms_version,attribution_snapshot)VALUES($1,$2,$3,$4,true,'dev-1','dev-1',$5)`, []any{member, brandIDs[b], user, joinMethod, selected}},
				{`INSERT INTO point_accounts(id,brand_id,brand_member_id)VALUES($1,$2,$3)`, []any{account, brandIDs[b], member}},
				{`INSERT INTO point_buckets(brand_id,account_id,source,state)SELECT $1,$2,s,state FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])state ON CONFLICT DO NOTHING`, []any{brandIDs[b], account}},
				{`INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at)VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, []any{ids.New(), hex.EncodeToString(digest[:]), user, member, brandIDs[b]}},
			} {
				if _, e := tx.Exec(ctx, q.SQL, q.Args...); e != nil {
					return e
				}
			}
			var delta points.Balance
			delta[0][0] = capacityFunding
			_, e := (points.Store{DB: p}).Post(ctx, tx, points.Change{BrandID: brandIDs[b], MemberID: member, EntryType: "adjustment", ReferenceType: "capacity_fixture", OperationKey: "capacity-fund-" + member, Reason: "synthetic isolated capacity funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: capacityFunding}}})
			return e
		})
		if agentJoins && i < brands {
			agentCodes[b] = capacityCommissionAgent(t, p, codes[b], member)
		}
		raw, e := json.Marshal(inputs[b])
		if e != nil {
			t.Fatal(e)
		}
		c := capacityUser{Brand: codes[b], Member: member, Token: token}
		response, e := f.request(ctx, c, "/bet-previews", "", raw)
		if e != nil {
			t.Fatal(e)
		}
		var envelope struct {
			Success bool `json:"success"`
			Data    struct {
				Actor string `json:"actor_context"`
			} `json:"data"`
		}
		if response.status != 200 || json.Unmarshal(response.body, &envelope) != nil || !envelope.Success || envelope.Data.Actor == "" {
			t.Fatal("capacity preview failed", response.status)
		}
		in := inputs[b]
		in.ActorContext = envelope.Data.Actor
		c.Body, e = json.Marshal(in)
		if e != nil {
			t.Fatal(e)
		}
		f.Users = append(f.Users, c)
	}
	return f
}

type capacityResponse struct {
	status int
	body   []byte
}

func (f capacityFixture) request(ctx context.Context, c capacityUser, path, key string, body []byte) (capacityResponse, error) {
	req, e := http.NewRequestWithContext(ctx, "POST", f.Server.URL+"/api/v1/b/"+c.Brand+path, bytes.NewReader(body))
	if e != nil {
		return capacityResponse{}, e
	}
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r, e := f.HTTP.Do(req)
	if e != nil {
		return capacityResponse{}, e
	}
	defer r.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return capacityResponse{status: r.StatusCode, body: raw}, e
}
func TestCapacityFixtureSmoke(t *testing.T) {
	f := newCapacityFixture(t, 2, 2, 20)
	ctx := context.Background()
	for i, c := range f.Users {
		key := "capacity-smoke-" + c.Member
		first, e := f.request(ctx, c, "/bet-orders", key, c.Body)
		if e != nil || first.status != 201 {
			t.Fatal("real capacity stake failed", i, first.status, e)
		}
		replay, e := f.request(ctx, c, "/bet-orders", key, c.Body)
		if e != nil || replay.status != 201 {
			t.Fatal("capacity replay failed", i, replay.status, e)
		}
		var a, b struct {
			Data betting.Order `json:"data"`
		}
		if json.Unmarshal(first.body, &a) != nil || json.Unmarshal(replay.body, &b) != nil || a.Data.ID == "" || a.Data.ID != b.Data.ID {
			t.Fatal("capacity receipt not stable")
		}
	}
	var orders, debits int
	if e := f.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet')`).Scan(&orders, &debits); e != nil || orders != 2 || debits != 2 {
		t.Fatal("capacity invariant", orders, debits, e)
	}
}

func TestCapacityCommissionMembershipSmoke(t *testing.T) {
	f := newCapacityMembers(t, 4, 2, 20, true)
	ctx := context.Background()
	var roots, children, codes int
	if err := f.DB.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM brand_members WHERE join_method='domain'),
	 (SELECT count(*) FROM brand_members WHERE join_method='agent_code' AND attribution_snapshot->>'agent_id' IS NOT NULL),
	 (SELECT count(*) FROM join_codes WHERE kind='agent')`).Scan(&roots, &children, &codes); err != nil {
		t.Fatal(err)
	}
	if roots != 2 || children != 2 || codes != 2 {
		t.Fatal("first-join agent provenance not captured", roots, children, codes)
	}
	prepareCapacityCommission(t, f)
	for i, user := range f.Users {
		r, err := f.request(ctx, user, "/bet-orders", "commission-member-smoke-"+user.Member, user.Body)
		if err != nil || r.status != 201 {
			t.Fatal("agent member bet failed", i, r.status, err)
		}
	}
	var withPath, entries int
	if err := f.DB.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM bet_orders WHERE jsonb_array_length(commission_rule_snapshot->'agent_path')=1),
	 (SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission')`).Scan(&withPath, &entries); err != nil {
		t.Fatal(err)
	}
	if withPath != 2 || entries != 0 {
		t.Fatal("bet-time agent path or unearned credit", withPath, entries)
	}
}
