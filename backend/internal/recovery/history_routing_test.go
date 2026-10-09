//go:build recovery && !windows

package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/httpapi"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type historyRoutingEvidence struct {
	SchemaVersion                  int    `json:"schema_version"`
	Scope                          string `json:"scope"`
	PostgresVersion                string `json:"postgres_version"`
	GoVersion                      string `json:"go_version"`
	ReplicaNodes                   int    `json:"replica_nodes"`
	PrimaryWriteVisibleOnBoth      bool   `json:"primary_write_visible_on_both"`
	RoundRobinReplicaIndexes       []int  `json:"round_robin_replica_indexes"`
	NotificationRouteReplica       int    `json:"notification_route_replica"`
	UnallowlistedRoutePrimaryOnly  bool   `json:"unallowlisted_route_primary_only"`
	PausedReplicaRejected          bool   `json:"paused_replica_rejected"`
	BothPausedRejected             bool   `json:"both_paused_rejected"`
	OfflineReplicaRejected         bool   `json:"offline_replica_rejected"`
	UnavailableOptionalPoolStartup bool   `json:"unavailable_optional_pool_startup"`
	WrongClusterAndPrimaryRejected bool   `json:"wrong_cluster_and_primary_rejected"`
	ReplicaQueryErrorReturned      bool   `json:"replica_query_error_returned"`
	HTTPHistoryRoutes              int    `json:"http_history_routes"`
	HTTPSuccessfulHistoryRequests  int    `json:"http_successful_history_requests"`
	HTTPObservedReplicaIndexes     []int  `json:"http_observed_replica_indexes"`
	CurrentTemplateHeadersAbsent   bool   `json:"current_template_headers_absent"`
	HTTPAuditFailureNoHistoryData  bool   `json:"http_audit_failure_no_history_data"`
	HTTPPermissionRevocation403    bool   `json:"http_permission_revocation_403"`
	HTTPSessionRevocation401       bool   `json:"http_session_revocation_401"`
	PrimaryFencePermissionError    bool   `json:"primary_fence_permission_error"`
	ReplicaQueryTimeoutError       bool   `json:"replica_query_timeout_error"`
	OriginalDigestBefore           string `json:"original_digest_before"`
	OriginalDigestAfter            string `json:"original_digest_after"`
	OriginalDatabaseUntouched      bool   `json:"original_database_untouched"`
	OwnedNodesStopped              bool   `json:"owned_nodes_stopped"`
	SyntheticMarkerRows            int    `json:"synthetic_marker_rows"`
	RecordedAtUTC                  string `json:"recorded_at_utc"`
}

func TestPhysicalHistoryReadRouting(t *testing.T) {
	root, bin, base, original := setupDrill(t)
	ctx := context.Background()
	originalBefore := snapshotDrill(t, original)

	nodes := make([]*drillNode, 4)
	for i, name := range []string{"history-primary", "history-replica1", "history-replica2", "history-unrelated"} {
		n := &drillNode{bin: bin, root: root, data: filepath.Join(root, name), name: name, port: base + i}
		nodes[i] = n
		t.Cleanup(func() {
			// A detached server may have started even if pg_ctl reported an error.
			status := exec.Command(filepath.Join(n.bin, "pg_ctl"), "-D", n.data, "status")
			status.Env = drillCommandEnvironment(os.Environ())
			if n.started || status.Run() == nil {
				n.started = true
				n.stop(t)
			}
		})
	}
	primary, replica1, replica2, unrelated := nodes[0], nodes[1], nodes[2], nodes[3]
	primary.command(t, "initdb", "-D", primary.data, "-U", "lottery_drill", "-A", "trust", "--locale=C", "--encoding=UTF8")
	primary.start(t, 0)
	primaryPool := primary.pool(t)
	_ = seedDrill(t, primaryPool)

	readerPools := make([]*pgxpool.Pool, 2)
	for i, node := range []*drillNode{replica1, replica2} {
		node.command(t, "pg_basebackup", "-D", node.data, "-h", "127.0.0.1", "-p", strconv.Itoa(primary.port), "-U", "lottery_drill", "-X", "stream", "-R", "-c", "fast")
		node.start(t, primary.port)
		readerPools[i] = node.pool(t)
	}
	// This independently initialized UTF8 node is deliberately writable and has
	// a distinct system identifier. It must never qualify as either standby.
	unrelated.command(t, "initdb", "-D", unrelated.data, "-U", "lottery_drill", "-A", "trust", "--locale=C", "--encoding=UTF8")
	unrelated.start(t, 0)
	unrelatedPool := unrelated.pool(t)
	var unrelatedRecovery bool
	if err := unrelatedPool.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&unrelatedRecovery); err != nil || unrelatedRecovery {
		t.Fatal("unrelated owned node is not a writable primary")
	}

	// A marker committed after both physical base backups provides the WAL fence
	// exercise and a single stable row for subsequent rotation requests.
	firstMarker := historyAuditMarker(t, primaryPool, "after_backup")
	for _, pool := range readerPools {
		waitHistoryAudit(t, pool, firstMarker)
	}
	evidence := historyRoutingEvidence{SchemaVersion: 1, Scope: "isolated loopback PostgreSQL physical history routing; synthetic HTTP administrator and audit rows only", ReplicaNodes: 2, PrimaryWriteVisibleOnBoth: true, OriginalDigestBefore: originalBefore.SHA256}
	router := database.NewHistoryRouter(readerPools)
	for _, expectedReplica := range []int{1, 2} {
		waitHistoryAuditReplica(t, primaryPool, readerPools[expectedReplica-1], router, firstMarker, expectedReplica)
		evidence.RoundRobinReplicaIndexes = append(evidence.RoundRobinReplicaIndexes, expectedReplica)
	}
	evidence.NotificationRouteReplica = waitHistoryValueReplica(t, primaryPool, readerPools[0], router, database.HistoryNotification, 1, func(tx pgx.Tx) (any, error) {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM notification_template_revisions`).Scan(&count)
		return count, err
	})
	_, unlistedSource, err := readHistoryValue(ctx, primaryPool, router, database.HistoryRoute("unlisted.history.route"), func(tx pgx.Tx) (any, error) {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action=$1`, firstMarker).Scan(&count)
		return count, err
	})
	if err != nil || unlistedSource.Replica != 0 || unlistedSource.Reason != "primary_only" {
		t.Fatal("unallowlisted history route was not kept on primary")
	}
	evidence.UnallowlistedRoutePrimaryOnly = true

	// Pause replica 1 and verify that the selected node's failure is returned.
	setReplayPaused(t, ctx, readerPools[0], true)
	pausedMarker := historyAuditMarker(t, primaryPool, "replica1_paused")
	waitHistoryAudit(t, readerPools[1], pausedMarker)
	pausedRouter := database.NewHistoryRouter(readerPools)
	_, _, err = readHistoryAudit(ctx, primaryPool, pausedRouter, pausedMarker)
	if err == nil {
		t.Fatal("paused selected replica did not return an error")
	}
	waitHistoryAuditReplica(t, primaryPool, readerPools[1], pausedRouter, pausedMarker, 2)
	evidence.PausedReplicaRejected = true

	setReplayPaused(t, ctx, readerPools[1], true)
	allPausedMarker := historyAuditMarker(t, primaryPool, "both_replicas_paused")
	allPausedRouter := database.NewHistoryRouter(readerPools)
	_, _, err = readHistoryAudit(ctx, primaryPool, allPausedRouter, allPausedMarker)
	if err == nil {
		t.Fatal("selected paused standby did not return an error")
	}
	_, _, err = readHistoryAudit(ctx, primaryPool, allPausedRouter, allPausedMarker)
	if err == nil {
		t.Fatal("second paused standby did not return an error")
	}
	evidence.BothPausedRejected = true

	setReplayPaused(t, ctx, readerPools[1], false)
	readerPools[0].Close()
	replica1.stop(t)
	openedPools, openErr := database.Open(ctx, config.Config{
		DatabaseURL:      fmt.Sprintf("postgres://lottery_drill@127.0.0.1:%d/postgres?sslmode=disable", primary.port),
		DatabaseReadURLs: []string{fmt.Sprintf("postgres://lottery_drill@127.0.0.1:%d/postgres?sslmode=disable", replica1.port)},
		DBMaxConns:       4,
	})
	if openErr != nil {
		t.Fatal("an unavailable optional history pool prevented primary database startup")
	}
	if err := openedPools.Primary.Ping(ctx); err != nil {
		openedPools.Close()
		t.Fatal("primary database was not available after optional-pool startup")
	}
	openedPools.Close()
	evidence.UnavailableOptionalPoolStartup = true
	offlineMarker := historyAuditMarker(t, primaryPool, "replica1_offline")
	waitHistoryAudit(t, readerPools[1], offlineMarker)
	offlineRouter := database.NewHistoryRouter(readerPools)
	_, _, err = readHistoryAudit(ctx, primaryPool, offlineRouter, offlineMarker)
	if err == nil {
		t.Fatal("offline selected replica did not return an error")
	}
	waitHistoryAuditReplica(t, primaryPool, readerPools[1], offlineRouter, offlineMarker, 2)
	evidence.OfflineReplicaRejected = true

	// Neither an unrelated writable cluster nor the actual writable primary can
	// pass the replica probes, even when both are configured as reader candidates.
	wrongNodesRouter := database.NewHistoryRouter([]*pgxpool.Pool{unrelatedPool, primaryPool})
	for i := 0; i < 2; i++ {
		if _, _, err = readHistoryAudit(ctx, primaryPool, wrongNodesRouter, offlineMarker); err == nil {
			t.Fatal("wrong-cluster or writable-primary candidate did not return an error")
		}
	}
	evidence.WrongClusterAndPrimaryRejected = true

	// Force a read-only SQL failure on the selected recovery node and verify it
	// is returned without retrying the callback on primary.
	queryErrorRouter := database.NewHistoryRouter([]*pgxpool.Pool{readerPools[1]})
	var value any
	var querySource database.ReadSource
	var readErr error
	queryCallbackRuns := 0
	waitDrill(t, func() bool {
		value, querySource, readErr = readHistoryValue(ctx, primaryPool, queryErrorRouter, database.HistoryAudit, func(tx pgx.Tx) (any, error) {
			queryCallbackRuns++
			var n int
			err := tx.QueryRow(ctx, `SELECT 1/(CASE WHEN pg_is_in_recovery() THEN 0 ELSE 1 END)`).Scan(&n)
			return n, err
		})
		return readErr != nil || querySource.Reason == "replica_query_failed"
	})
	if readErr == nil || value != nil || queryCallbackRuns != 1 || querySource.Replica != 1 || querySource.Reason != "replica_query_failed" {
		t.Fatalf("replica query failure was not returned (callback_runs=%d replica=%d reason=%s)", queryCallbackRuns, querySource.Replica, querySource.Reason)
	}
	var audits int
	if err := primaryPool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action LIKE 'recovery.history.routing.%'`).Scan(&audits); err != nil || audits != 4 {
		t.Fatal("routing drill changed or lost synthetic audit rows")
	}
	evidence.ReplicaQueryErrorReturned = true
	evidence.SyntheticMarkerRows = audits

	// Reattach the offline standby so the HTTP handler can be observed routing
	// through both physical replicas as its own audit writes advance the WAL.
	replica1.start(t, primary.port)
	readerPools[0] = replica1.pool(t)
	setReplayPaused(t, ctx, readerPools[0], false)
	waitHistoryAudit(t, readerPools[0], offlineMarker)
	httpRouter := database.NewHistoryRouter(readerPools)
	httpEvidence := runHistoryHTTPDrill(t, primaryPool, readerPools, httpRouter)
	evidence.HTTPHistoryRoutes = httpEvidence.routes
	evidence.HTTPSuccessfulHistoryRequests = httpEvidence.successful
	evidence.HTTPObservedReplicaIndexes = httpEvidence.replicas
	evidence.CurrentTemplateHeadersAbsent = httpEvidence.currentTemplateHeadersAbsent
	evidence.HTTPAuditFailureNoHistoryData = httpEvidence.auditFailureNoData
	evidence.HTTPPermissionRevocation403 = httpEvidence.permissionRevocation403
	evidence.HTTPSessionRevocation401 = httpEvidence.sessionRevocation401
	evidence.PrimaryFencePermissionError = httpEvidence.fencePermissionError
	evidence.ReplicaQueryTimeoutError = httpEvidence.timeoutError
	evidence.SyntheticMarkerRows += 3 // Timeout, withheld-response, and paused-primary markers.

	// Cleanly close all pools, resume paused replay, and stop every owned server
	// before taking the final read-only snapshot of the original database.
	setReplayPaused(t, ctx, readerPools[1], false)
	primaryPool.Close()
	readerPools[0].Close()
	readerPools[1].Close()
	unrelatedPool.Close()
	unrelated.stop(t)
	replica1.stop(t)
	replica2.stop(t)
	primary.stop(t)
	assertHistoryDrillNodesStopped(t, nodes)
	originalAfter := snapshotDrill(t, original)
	evidence.OriginalDigestAfter = originalAfter.SHA256
	evidence.OriginalDatabaseUntouched = originalBefore.SHA256 == originalAfter.SHA256
	evidence.OwnedNodesStopped = true
	if !evidence.OriginalDatabaseUntouched {
		t.Fatal("read-only original database snapshot changed")
	}
	if err := writeHistoryRoutingEvidence(t, bin, evidence); err != nil {
		t.Fatal(err)
	}
	t.Logf("replicas=%d round_robin=%v paused_rejected=%t offline_rejected=%t original_untouched=%t", evidence.ReplicaNodes, evidence.RoundRobinReplicaIndexes, evidence.PausedReplicaRejected, evidence.OfflineReplicaRejected, evidence.OriginalDatabaseUntouched)
}

func assertHistoryDrillNodesStopped(t *testing.T, nodes []*drillNode) {
	t.Helper()
	for _, node := range nodes {
		cmd := exec.Command(filepath.Join(node.bin, "pg_ctl"), "-D", node.data, "status")
		cmd.Env = drillCommandEnvironment(os.Environ())
		err := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
			t.Fatalf("owned PostgreSQL node %s did not confirm stopped status", node.name)
		}
	}
}

func historyAuditMarker(t *testing.T, pool *pgxpool.Pool, label string) string {
	t.Helper()
	action := "recovery.history.routing." + label + "." + ids.New()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO audit_logs(id,actor_type,action,resource_type,request_id) VALUES($1,'system',$2,'history_routing_drill',$2)`, ids.New(), action); err != nil {
		t.Fatal("could not commit synthetic history marker")
	}
	return action
}

func waitHistoryAudit(t *testing.T, pool *pgxpool.Pool, action string) {
	t.Helper()
	waitDrill(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var count int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action=$1`, action).Scan(&count)
		return err == nil && count == 1
	})
}

func waitHistoryFence(t *testing.T, primary, replica *pgxpool.Pool) {
	t.Helper()
	var fence string
	if err := primary.QueryRow(context.Background(), `SELECT pg_current_wal_insert_lsn()::text`).Scan(&fence); err != nil {
		t.Fatal(err)
	}
	// A visible marker does not prove later WAL has replayed. Synchronize the
	// owned test node explicitly, without asking the application to fall back.
	waitDrill(t, func() bool {
		var ready bool
		err := replica.QueryRow(context.Background(), `SELECT coalesce(pg_last_wal_replay_lsn() >= $1::pg_lsn,false)`, fence).Scan(&ready)
		return err == nil && ready
	})
}

func waitHistoryAuditReplica(t *testing.T, primary, replica *pgxpool.Pool, router *database.HistoryRouter, action string, expectedReplica int) {
	t.Helper()
	waitHistoryFence(t, primary, replica)
	var count int
	var source database.ReadSource
	var readErr error
	waitDrill(t, func() bool {
		count, source, readErr = readHistoryAudit(context.Background(), primary, router, action)
		if readErr != nil && source.Reason == "replica_not_wal_fenced" {
			return false // Owned streaming node has not caught up to this sampled fence yet.
		}
		return readErr != nil || (source.Replica == expectedReplica && source.Reason == "wal_fenced")
	})
	if readErr != nil || count != 1 || source.Replica != expectedReplica || source.Reason != "wal_fenced" {
		var primaryState, replicaState string
		_ = primary.QueryRow(context.Background(), `SELECT json_build_object('system',(pg_control_system()).system_identifier::text,'database',current_database(),'schemas',current_schemas(true),'lsn',pg_current_wal_insert_lsn()::text,'timeline',substring(pg_walfile_name(pg_current_wal_insert_lsn()),1,8))::text`).Scan(&primaryState)
		_ = replica.QueryRow(context.Background(), `SELECT json_build_object('system',(pg_control_system()).system_identifier::text,'database',current_database(),'schemas',current_schemas(true),'lsn',pg_last_wal_replay_lsn()::text,'timeline',(pg_control_checkpoint()).timeline_id,'recovery_timeline',(pg_control_recovery()).min_recovery_end_timeline,'recovery',pg_is_in_recovery(),'paused',pg_is_wal_replay_paused())::text`).Scan(&replicaState)
		t.Logf("owned fence diagnostics: primary=%s replica=%s", primaryState, replicaState)
		t.Fatalf("WAL-fenced audit read mismatch: expected replica %d, got replica=%d reason=%s count=%d", expectedReplica, source.Replica, source.Reason, count)
	}
}

func waitHistoryValueReplica(t *testing.T, primary, replica *pgxpool.Pool, router *database.HistoryRouter, route database.HistoryRoute, expectedReplica int, run func(pgx.Tx) (any, error)) int {
	t.Helper()
	waitHistoryFence(t, primary, replica)
	var value any
	var source database.ReadSource
	var readErr error
	waitDrill(t, func() bool {
		value, source, readErr = readHistoryValue(context.Background(), primary, router, route, run)
		if readErr != nil && source.Reason == "replica_not_wal_fenced" {
			return false
		}
		return readErr != nil || (source.Replica == expectedReplica && source.Reason == "wal_fenced")
	})
	if readErr != nil || source.Replica != expectedReplica || source.Reason != "wal_fenced" {
		t.Fatalf("WAL-fenced history read mismatch: expected replica %d, got replica=%d reason=%s", expectedReplica, source.Replica, source.Reason)
	}
	if _, ok := value.(int); !ok {
		t.Fatal("notification history callback returned an unexpected result")
	}
	return source.Replica
}

func waitHistoryRequestAudit(t *testing.T, pool *pgxpool.Pool, requestID string) {
	t.Helper()
	waitDrill(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_logs WHERE request_id=$1)`, requestID).Scan(&exists)
		return err == nil && exists
	})
}

func readHistoryAudit(ctx context.Context, primary *pgxpool.Pool, router *database.HistoryRouter, action string) (int, database.ReadSource, error) {
	value, source, err := readHistoryValue(ctx, primary, router, database.HistoryAudit, func(tx pgx.Tx) (any, error) {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action=$1`, action).Scan(&count)
		return count, err
	})
	if err != nil {
		return 0, source, err
	}
	count, ok := value.(int)
	if !ok {
		return 0, source, fmt.Errorf("history audit callback returned an unexpected result")
	}
	return count, source, nil
}

func readHistoryValue(ctx context.Context, primary *pgxpool.Pool, router *database.HistoryRouter, route database.HistoryRoute, run func(pgx.Tx) (any, error)) (any, database.ReadSource, error) {
	tx, err := primary.Begin(ctx)
	if err != nil {
		return nil, database.ReadSource{}, err
	}
	defer tx.Rollback(ctx)
	value, source, err := router.Read(ctx, tx, route, run)
	if err != nil {
		return nil, source, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, source, err
	}
	return value, source, nil
}

func setReplayPaused(t *testing.T, ctx context.Context, pool *pgxpool.Pool, paused bool) {
	t.Helper()
	fn := "pg_wal_replay_resume()"
	if paused {
		fn = "pg_wal_replay_pause()"
	}
	if _, err := pool.Exec(ctx, "SELECT "+fn); err != nil {
		t.Fatal("could not change replay state on an owned standby")
	}
	waitDrill(t, func() bool {
		var got bool
		probeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := pool.QueryRow(probeCtx, `SELECT pg_is_wal_replay_paused()`).Scan(&got)
		return err == nil && got == paused
	})
}

type historyHTTPDrillEvidence struct {
	routes, successful                            int
	replicas                                      []int
	currentTemplateHeadersAbsent                  bool
	auditFailureNoData                            bool
	permissionRevocation403, sessionRevocation401 bool
	fencePermissionError, timeoutError            bool
}

func runHistoryHTTPDrill(t *testing.T, primary *pgxpool.Pool, replicas []*pgxpool.Pool, router *database.HistoryRouter) historyHTTPDrillEvidence {
	t.Helper()
	ctx := context.Background()
	users, err := identity.New(primary)
	if err != nil {
		t.Fatal("could not initialize synthetic HTTP identity store")
	}
	const password = "history-routing-drill-password-2026"
	hash, err := users.PasswordHash(ctx, password)
	if err != nil {
		t.Fatal("could not hash synthetic HTTP administrator password")
	}
	adminID, roleID := ids.New(), ids.New()
	for i, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'history_route_admin',$2)`, []any{adminID, hash}},
		{`INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,'history_route_admin','History routing drill',true)`, []any{roleID, drillBrand}},
		{`INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, []any{adminID, drillBrand}},
		{`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{adminID, roleID}},
	} {
		if _, err := primary.Exec(ctx, statement.sql, statement.args...); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				t.Fatalf("synthetic HTTP administrator fixture statement %d failed (SQLSTATE %s, constraint %s)", i+1, pgErr.Code, pgErr.ConstraintName)
			}
			t.Fatalf("synthetic HTTP administrator fixture statement %d failed", i+1)
		}
	}
	permissions := []string{
		"audit.view.brand",
		"brand_presentation.view.brand",
		"brand_domains.view.brand",
		"brand_operation.view.brand",
		"compliance_policy.view.brand",
		"notification_template.view.brand",
	}
	for _, permission := range permissions {
		if _, err := primary.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal("could not create synthetic history permission")
		}
		if _, err := primary.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, permission); err != nil {
			t.Fatal("could not grant synthetic history permission")
		}
	}
	mutations, err := mutation.New(primary, make([]byte, 32))
	if err != nil {
		t.Fatal("could not initialize synthetic HTTP mutation engine")
	}
	handler := httpapi.New(httpapi.Dependencies{
		Brands:       tenant.Store{DB: primary},
		Ready:        primary.Ping,
		Identity:     users,
		Mutations:    mutations,
		Admins:       adminsys.Store{DB: primary},
		HistoryReads: router,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	login := historyHTTPCall(t, handler, http.MethodPost, "/api/v1/admin/auth/login", "history-routing-login-001", "", map[string]string{
		"identifier": "history_route_admin",
		"password":   password,
	})
	if login.Code != http.StatusOK {
		t.Fatal("synthetic administrator could not log in through HTTP")
	}
	var loginEnvelope struct {
		Data identity.AdminAuthentication `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginEnvelope); err != nil || loginEnvelope.Data.AccessToken == "" {
		t.Fatal("HTTP login did not return a usable synthetic session")
	}
	token := loginEnvelope.Data.AccessToken

	paths := []string{
		"/api/v1/admin/audit",
		"/api/v1/admin/brand-presentation/history",
		"/api/v1/admin/brand-domains/history",
		"/api/v1/admin/brand-operation/history",
		"/api/v1/admin/compliance-policy/history",
		"/api/v1/admin/notification-templates/member.joined/history",
	}
	evidence := historyHTTPDrillEvidence{routes: len(paths)}
	seenReplica := map[int]bool{}
	observe := func(response *httptest.ResponseRecorder) {
		t.Helper()
		if response.Code != http.StatusOK {
			t.Fatal("synthetic history endpoint did not return HTTP 200")
		}
		requestID := response.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Fatal("history response omitted its request identifier")
		}
		for _, replica := range replicas {
			waitHistoryRequestAudit(t, replica, requestID)
		}
		source, reason := response.Header().Get("X-Read-Source"), response.Header().Get("X-Read-Reason")
		if reason == "" || (source != "primary" && source != "replica") {
			t.Fatal("history endpoint omitted its fixed read-source evidence")
		}
		index, err := strconv.Atoi(response.Header().Get("X-Read-Replica"))
		if err != nil || source != "replica" || index < 1 || index > len(replicas) || reason != "wal_fenced" {
			t.Fatal("history endpoint returned invalid physical-replica evidence")
		}
		seenReplica[index] = true
	}
	for _, path := range paths {
		var response *httptest.ResponseRecorder
		waitDrill(t, func() bool {
			response = historyHTTPCall(t, handler, http.MethodGet, path, "", token, nil)
			if response.Code == http.StatusServiceUnavailable && historyHTTPHeadersAbsent(response) && !historyHTTPHasData(response) {
				return false
			}
			return true
		})
		observe(response)
		evidence.successful++
	}

	current := historyHTTPCall(t, handler, http.MethodGet, "/api/v1/admin/notification-templates", "", token, nil)
	if current.Code != http.StatusOK || !historyHTTPHeadersAbsent(current) {
		t.Fatal("current notification-template GET incorrectly exposed history-routing headers")
	}
	if requestID := current.Header().Get("X-Request-ID"); requestID != "" {
		for _, replica := range replicas {
			waitHistoryRequestAudit(t, replica, requestID)
		}
	}
	evidence.currentTemplateHeadersAbsent = true

	// Each history request itself appends an audit row, so wait for replay before
	// retrying the next selected replica.
	for attempt := 0; attempt < 60 && (!seenReplica[1] || !seenReplica[2]); attempt++ {
		time.Sleep(75 * time.Millisecond)
		response := historyHTTPCall(t, handler, http.MethodGet, paths[attempt%len(paths)], "", token, nil)
		if response.Code == http.StatusServiceUnavailable && historyHTTPHeadersAbsent(response) && !historyHTTPHasData(response) {
			continue
		}
		observe(response)
		evidence.successful++
	}
	if !seenReplica[1] || !seenReplica[2] {
		t.Fatal("HTTP history requests did not observe both configured physical replicas")
	}
	for i := 1; i <= len(replicas); i++ {
		if seenReplica[i] {
			evidence.replicas = append(evidence.replicas, i)
		}
	}

	// Test the router's fixed two-second query cap even when the callback passes
	// context.Background. The selected replica query times out and returns error.
	timeoutMarker := historyAuditMarker(t, primary, "http_timeout_fence")
	waitHistoryAudit(t, replicas[1], timeoutMarker)
	var timeoutEligible, timeoutVerified bool
	waitDrill(t, func() bool {
		timeoutEligible, timeoutVerified = verifyHistoryReplicaQueryTimeout(t, primary, replicas[1])
		return timeoutEligible
	})
	if !timeoutVerified {
		t.Fatal("replica query timeout was not returned explicitly")
	}
	evidence.timeoutError = true

	if !verifyHistoryFencePermissionError(t, primary, router, timeoutMarker) {
		t.Fatal("primary fence privilege failure was not returned after savepoint rollback")
	}
	evidence.fencePermissionError = true

	leakMarker := historyHTTPAuditMarker(t, primary, "audit_failure_no_response_"+ids.New())
	installHistoryAuditFailure(t, primary)
	failureResponse := historyHTTPCall(t, handler, http.MethodGet, paths[0], "", token, nil)
	if failureResponse.Code != http.StatusServiceUnavailable || !historyHTTPHeadersAbsent(failureResponse) || historyHTTPHasData(failureResponse) || bytes.Contains(failureResponse.Body.Bytes(), []byte(leakMarker)) {
		removeHistoryAuditFailure(primary)
		t.Fatal("failed history audit released response data or routing headers")
	}
	removeHistoryAuditFailure(primary)
	evidence.auditFailureNoData = true

	setReplayPaused(t, ctx, replicas[0], true)
	setReplayPaused(t, ctx, replicas[1], true)
	pausedMarker := historyHTTPAuditMarker(t, primary, "http_both_paused_"+ids.New())
	pausedResponse := historyHTTPCall(t, handler, http.MethodGet, paths[0], "", token, nil)
	if pausedResponse.Code != http.StatusServiceUnavailable || !historyHTTPHeadersAbsent(pausedResponse) || historyHTTPHasData(pausedResponse) || bytes.Contains(pausedResponse.Body.Bytes(), []byte(pausedMarker)) {
		t.Fatal("paused selected replica failure did not withhold history data")
	}
	setReplayPaused(t, ctx, replicas[0], false)
	setReplayPaused(t, ctx, replicas[1], false)
	waitHistoryAudit(t, replicas[0], pausedMarker)
	waitHistoryAudit(t, replicas[1], pausedMarker)

	if _, err := primary.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='audit.view.brand'`, roleID); err != nil {
		t.Fatal("could not revoke synthetic audit permission")
	}
	permissionResponse := historyHTTPCall(t, handler, http.MethodGet, paths[0], "", token, nil)
	if permissionResponse.Code != http.StatusForbidden || !historyHTTPHeadersAbsent(permissionResponse) {
		t.Fatal("freshly revoked history permission was not enforced by HTTP")
	}
	evidence.permissionRevocation403 = true
	if _, err := primary.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE admin_id=$1 AND revoked_at IS NULL`, adminID); err != nil {
		t.Fatal("could not revoke synthetic HTTP administrator session")
	}
	sessionResponse := historyHTTPCall(t, handler, http.MethodGet, "/api/v1/admin/notification-templates", "", token, nil)
	if sessionResponse.Code != http.StatusUnauthorized || !historyHTTPHeadersAbsent(sessionResponse) {
		t.Fatal("freshly revoked administrator session was not enforced by HTTP")
	}
	evidence.sessionRevocation401 = true
	return evidence
}

func historyHTTPCall(t *testing.T, handler http.Handler, method, path, idemKey, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal("could not encode synthetic HTTP request")
		}
	}
	request := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(raw))
	request.Header.Set("X-Brand-ID", drillBrand)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		request.Header.Set("Idempotency-Key", idemKey)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func historyHTTPHeadersAbsent(response *httptest.ResponseRecorder) bool {
	return response.Header().Get("X-Read-Source") == "" && response.Header().Get("X-Read-Reason") == "" && response.Header().Get("X-Read-Replica") == ""
}

func historyHTTPHasData(response *httptest.ResponseRecorder) bool {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(response.Body.Bytes(), &envelope) != nil {
		return true
	}
	return len(envelope.Data) != 0 && !bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null"))
}

func historyHTTPAuditMarker(t *testing.T, pool *pgxpool.Pool, action string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,request_id) VALUES($1,$2,'system',$3,'history_routing_http',$3)`, ids.New(), drillBrand, action); err != nil {
		t.Fatal("could not write synthetic brand audit marker")
	}
	return action
}

func installHistoryAuditFailure(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE FUNCTION public.history_routing_reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='audit.view' THEN RAISE EXCEPTION 'synthetic history audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal("could not install owned audit-failure trigger")
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER history_routing_reject_audit BEFORE INSERT ON public.audit_logs FOR EACH ROW EXECUTE FUNCTION public.history_routing_reject_audit()`); err != nil {
		_, _ = pool.Exec(ctx, `DROP FUNCTION IF EXISTS public.history_routing_reject_audit()`)
		t.Fatal("could not install owned audit-failure trigger")
	}
	t.Cleanup(func() { removeHistoryAuditFailure(pool) })
}

func removeHistoryAuditFailure(pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = pool.Exec(ctx, `DROP TRIGGER IF EXISTS history_routing_reject_audit ON public.audit_logs`)
	_, _ = pool.Exec(ctx, `DROP FUNCTION IF EXISTS public.history_routing_reject_audit()`)
}

func verifyHistoryFencePermissionError(t *testing.T, primary *pgxpool.Pool, router *database.HistoryRouter, action string) bool {
	t.Helper()
	ctx := context.Background()
	if _, err := primary.Exec(ctx, `CREATE ROLE history_routing_limited_probe NOLOGIN`); err != nil {
		t.Fatal("could not create owned least-privilege probe role")
	}
	defer func() {
		_, _ = primary.Exec(context.Background(), `GRANT EXECUTE ON FUNCTION pg_catalog.pg_control_system() TO PUBLIC`)
		_, _ = primary.Exec(context.Background(), `REVOKE ALL PRIVILEGES ON public.audit_logs FROM history_routing_limited_probe`)
		_, _ = primary.Exec(context.Background(), `DROP ROLE IF EXISTS history_routing_limited_probe`)
	}()
	if _, err := primary.Exec(ctx, `GRANT SELECT ON public.audit_logs TO history_routing_limited_probe`); err != nil {
		t.Fatal("could not grant owned probe table read")
	}
	if _, err := primary.Exec(ctx, `REVOKE EXECUTE ON FUNCTION pg_catalog.pg_control_system() FROM PUBLIC`); err != nil {
		t.Fatal("could not restrict owned control-function execute privilege")
	}
	tx, err := primary.Begin(ctx)
	if err != nil {
		t.Fatal("could not begin restricted primary read")
	}
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE history_routing_limited_probe`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("could not assume owned least-privilege probe role")
	}
	_, source, readErr := router.Read(ctx, tx, database.HistoryAudit, func(readTx pgx.Tx) (any, error) {
		var count int
		err := readTx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action=$1`, action).Scan(&count)
		return count, err
	})
	if readErr == nil || source.Reason != "fence_unavailable" {
		_ = tx.Rollback(ctx)
		t.Fatal("primary fence privilege failure was not returned")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("outer primary transaction could not commit after fence savepoint rollback")
	}
	return source.Replica == 0 && source.Reason == "fence_unavailable"
}

func verifyHistoryReplicaQueryTimeout(t *testing.T, primary, replica *pgxpool.Pool) (bool, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	primaryTx, err := primary.Begin(ctx)
	if err != nil {
		t.Fatal("could not begin query-timeout primary transaction")
	}
	defer primaryTx.Rollback(ctx)
	router := database.NewHistoryRouter([]*pgxpool.Pool{replica})
	callbackRuns := 0
	started := time.Now()
	value, source, err := router.Read(ctx, primaryTx, database.HistoryAudit, func(tx pgx.Tx) (any, error) {
		callbackRuns++
		var result int
		err := tx.QueryRow(context.Background(), `SELECT CASE WHEN pg_is_in_recovery() THEN (SELECT count(*) FROM pg_sleep(5)) ELSE 0 END`).Scan(&result)
		return result, err
	})
	elapsed := time.Since(started)
	if err := primaryTx.Commit(ctx); err != nil {
		t.Fatal("query-timeout primary transaction did not commit after replica error")
	}
	eligible := err != nil && source.Reason == "replica_query_failed"
	if !eligible {
		return false, false
	}
	verified := value == nil && callbackRuns == 1 && source.Replica == 1 && elapsed >= 1800*time.Millisecond && elapsed < 5*time.Second
	return true, verified
}

func writeHistoryRoutingEvidence(t *testing.T, bin string, evidence historyRoutingEvidence) error {
	t.Helper()
	path := os.Getenv("LOTTERY_RECOVERY_REPORT")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("new absolute history-routing report path required")
	}
	version, err := exec.Command(filepath.Join(bin, "postgres"), "--version").Output()
	if err != nil {
		return fmt.Errorf("cannot read owned PostgreSQL tool version")
	}
	evidence.PostgresVersion = strings.TrimSpace(string(version))
	evidence.GoVersion = runtime.Version()
	evidence.RecordedAtUTC = time.Now().UTC().Format(time.RFC3339)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot exclusively create history-routing evidence report")
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot set history-routing evidence report permissions")
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(evidence)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		return fmt.Errorf("cannot persist history-routing evidence report")
	}
	return nil
}
