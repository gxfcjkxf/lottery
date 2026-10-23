// Package adminsys provides persistent administrative account authorization
// and brand-scoped member operations.
package adminsys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDenied          = errors.New("administrative action denied")
	ErrNotFound        = errors.New("administrative resource not found")
	ErrInvalid         = errors.New("invalid administrative request")
	ErrCredentialScope = fmt.Errorf("%w: shared credentials require every member brand", ErrDenied)
)

// Store uses the application's PostgreSQL pool. Mutations participate in the
// transaction supplied by the caller so the data change and audit entry commit
// or roll back together.
type Store struct{ DB *pgxpool.Pool }

// Member is a brand membership joined with its global identity. Tags are read
// from profile_snapshot.tags when that value is a JSON array.
type Member struct {
	ID            string    `json:"id"`
	GlobalUserID  string    `json:"global_user_id"`
	Username      string    `json:"username"`
	Phone         string    `json:"phone"`
	DisplayName   string    `json:"display_name"`
	Notes         string    `json:"notes"`
	Status        string    `json:"status"`
	JoinedAt      time.Time `json:"joined_at"`
	BrandID       string    `json:"brand_id"`
	Tags          []string  `json:"tags"`
	Online        bool      `json:"online"`
	DisplayPoints *string   `json:"display_points"`
}

// Account loads an active admin's exact role permissions and brand scopes.
func (s Store) Account(ctx context.Context, accountID string) (access.Account, error) {
	return loadAccount(ctx, s.DB, accountID)
}

type accountQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// This shared/exclusive gate serializes management revocations against
// administrative writes. It is not used for the high-throughput betting path.
const adminAccessLock int64 = 0x6c6f747461636c

func (s Store) LockAdminAccess(ctx context.Context, tx pgx.Tx, id string, exclusive bool) (access.Account, error) {
	query := "SELECT pg_advisory_xact_lock_shared($1)"
	if exclusive {
		query = "SELECT pg_advisory_xact_lock($1)"
	}
	if _, err := tx.Exec(ctx, query, adminAccessLock); err != nil {
		return access.Account{}, err
	}
	return loadAccount(ctx, tx, id)
}

func ManagementAllowed(a access.Account, resource, action, brand string) bool {
	return access.Authorize(a, resource, action, access.ScopeBrand, brand) ||
		access.Authorize(a, resource, action, access.ScopePlatform, "")
}

func loadAccount(ctx context.Context, q accountQuery, accountID string) (access.Account, error) {
	var account access.Account
	var status string
	err := q.QueryRow(ctx, `SELECT id::text,username,status,is_super_admin,version FROM admin_accounts WHERE id=$1`, accountID).
		Scan(&account.ID, &account.Username, &status, &account.SuperAdmin, &account.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.Account{}, ErrNotFound
	}
	if err != nil {
		return access.Account{}, err
	}
	if status != "active" {
		return access.Account{}, ErrDenied
	}
	account.Type = access.AccountAdmin

	rows, err := q.Query(ctx, `SELECT r.id::text,coalesce(r.brand_id::text,''),p.key
		FROM admin_account_roles ar
		JOIN roles r ON r.id=ar.role_id
		JOIN role_permissions rp ON rp.role_id=r.id
		JOIN permissions p ON p.key=rp.permission_key
		WHERE ar.account_id=$1 AND r.status='active'
		ORDER BY r.id,p.key`, accountID)
	if err != nil {
		return access.Account{}, err
	}
	roles := make(map[string]*access.Role)
	for rows.Next() {
		var roleID, brand, key string
		if err := rows.Scan(&roleID, &brand, &key); err != nil {
			rows.Close()
			return access.Account{}, err
		}
		resource, action, scope, ok := parsePermission(key)
		if !ok {
			continue
		}
		role := roles[roleID]
		if role == nil {
			role = &access.Role{BrandID: brand}
			roles[roleID] = role
		}
		role.Permissions = append(role.Permissions, access.Permission{Resource: resource, Action: action, Scope: scope})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return access.Account{}, err
	}
	rows.Close()
	roleIDs := make([]string, 0, len(roles))
	for roleID := range roles {
		roleIDs = append(roleIDs, roleID)
	}
	sort.Strings(roleIDs)
	for _, roleID := range roleIDs {
		account.Roles = append(account.Roles, *roles[roleID])
	}

	rows, err = q.Query(ctx, `SELECT brand_id::text FROM admin_brand_scopes WHERE account_id=$1 ORDER BY brand_id`, accountID)
	if err != nil {
		return access.Account{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var brandID string
		if err := rows.Scan(&brandID); err != nil {
			return access.Account{}, err
		}
		account.BrandIDs = append(account.BrandIDs, brandID)
	}
	if err := rows.Err(); err != nil {
		return access.Account{}, err
	}
	return account, nil
}

// ListMembers requires user.view.platform (which may list any or all brands)
// or user.view.brand plus membership in the requested brand.
type MemberFilter struct {
	Username string
	Search   string
	Online   *bool
}
type MemberPage struct {
	Items      []Member `json:"items"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	TotalCount string   `json:"total_count"`
}

func (s Store) ListMembers(ctx context.Context, account access.Account, brandID string, filter MemberFilter, limit, offset int) (MemberPage, error) {
	out := MemberPage{Items: []Member{}, Limit: limit, Offset: offset}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	platform := access.Authorize(account, "user", "view", access.ScopePlatform, "")
	brandView := brandID != "" && access.Authorize(account, "user", "view", access.ScopeBrand, brandID)
	if !platform && !brandView {
		return out, ErrDenied
	}
	walletPlatform := access.Authorize(account, "wallet", "view", access.ScopePlatform, "")
	walletBrands := []string{}
	for _, id := range account.BrandIDs {
		if access.Authorize(account, "wallet", "view", access.ScopeBrand, id) {
			walletBrands = append(walletBrands, id)
		}
	}
	var raw []byte
	err := s.DB.QueryRow(ctx, `WITH candidates AS (
 SELECT bm.id::text,bm.global_user_id::text,COALESCE(gu.username,'') AS username,COALESCE(gu.phone,'') AS phone,
 bm.display_name,bm.notes,bm.status,bm.joined_at,bm.brand_id::text,
 CASE WHEN jsonb_typeof(bm.profile_snapshot->'tags')='array' THEN bm.profile_snapshot->'tags' ELSE '[]'::jsonb END AS tags,
 EXISTS(SELECT 1 FROM sessions se JOIN session_activity sa ON sa.session_id=se.id
 JOIN brands br ON br.id=se.brand_id
 WHERE se.member_id=bm.id AND se.brand_id=bm.brand_id AND se.user_id=bm.global_user_id
 AND sa.last_seen_at>=now()-interval '5 minutes' AND se.revoked_at IS NULL AND se.expires_at>now()
 AND gu.status='active' AND bm.status IN ('normal','frozen') AND bm.terms_accepted AND br.status<>'disabled') AS online,
 CASE WHEN $8::boolean OR bm.brand_id=ANY($9::uuid[]) THEN
 (SELECT COALESCE(sum(pb.points),0)::text FROM point_accounts pa JOIN point_buckets pb ON pb.account_id=pa.id AND pb.brand_id=pa.brand_id
 WHERE pa.brand_id=bm.brand_id AND pa.brand_member_id=bm.id AND pb.state IN ('available','manual_frozen','system_frozen')) ELSE NULL END AS display_points
 FROM brand_members bm JOIN global_users gu ON gu.id=bm.global_user_id
 WHERE ($1='' OR bm.brand_id=NULLIF($1,'')::uuid) AND bm.status<>'cancelled'
 AND ($4::boolean OR EXISTS(SELECT 1 FROM admin_brand_scopes sc WHERE sc.account_id=$5 AND sc.brand_id=bm.brand_id))
 AND ($6='' OR lower(gu.username)=lower($6))
 AND ($10='' OR strpos(lower(concat_ws(' ',gu.username,gu.phone,bm.display_name)),lower($10))>0)
 ), selected AS (SELECT * FROM candidates WHERE $7::boolean IS NULL OR online=$7),
 page AS (SELECT * FROM selected ORDER BY joined_at DESC,id LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY joined_at DESC,id) FROM page),'[]'::jsonb)`,
		brandID, limit, offset, platform, account.ID, filter.Username, filter.Online, walletPlatform, walletBrands, filter.Search).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, fmt.Errorf("decode member page: %w", err)
	}
	return out, nil
}

// ChangeMember changes only a membership in the specified brand. Snapshots
// contain status alone; identity, credentials, notes, and contact details are
// omitted from the audit record.
func (s Store) ChangeMember(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, status, notes, reason, requestID, ip string) (string, error) {
	if !validStatus(status) || tx == nil {
		return "", ErrInvalid
	}
	if !access.Authorize(account, "user", "write", access.ScopeBrand, brandID) {
		return "", ErrDenied
	}
	before, err := lockMember(ctx, tx, brandID, memberID)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE brand_members SET status=$3,notes=$4 WHERE brand_id=$1 AND id=$2`, brandID, memberID, status, notes); err != nil {
		return "", err
	}
	return audit.Append(ctx, tx, audit.Record{
		BrandID: brandID, ActorType: "admin", ActorID: account.ID, Action: "user.write",
		ResourceType: "brand_member", ResourceID: memberID, Reason: reason, RequestID: requestID, IP: ip,
		Before: map[string]string{"status": before.Status}, After: map[string]string{"status": status},
	})
}

// DeleteMember cancels a membership in the selected brand and revokes its
// sessions. Historical transactions and the global identity remain intact.
func (s Store) DeleteMember(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, requestID, ip string) (string, error) {
	if tx == nil || strings.TrimSpace(brandID) == "" || strings.TrimSpace(memberID) == "" {
		return "", ErrInvalid
	}
	if account.SuperAdmin {
		return "", ErrDenied
	}
	brandPermission := access.Authorize(account, "user", "delete", access.ScopeBrand, brandID)
	platformPermission := access.Authorize(account, "user", "delete", access.ScopePlatform, "")
	if !brandPermission && !platformPermission {
		return "", ErrDenied
	}
	before, err := lockMember(ctx, tx, brandID, memberID)
	if err != nil {
		return "", err
	}
	if before.Status == "cancelled" {
		return "", ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE brand_members SET status='cancelled' WHERE brand_id=$1 AND id=$2`, brandID, memberID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE brand_id=$1 AND member_id=$2 AND revoked_at IS NULL`, brandID, memberID); err != nil {
		return "", err
	}
	return audit.Append(ctx, tx, audit.Record{
		BrandID: brandID, ActorType: "admin", ActorID: account.ID, Action: "user.delete",
		ResourceType: "brand_member", ResourceID: memberID, RequestID: requestID, IP: ip,
		Before: map[string]string{"status": before.Status}, After: map[string]string{"status": "cancelled"},
	})
}

// Kick revokes only sessions for the selected member in the selected brand.
func (s Store) Kick(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, reason, requestID, ip string) (string, error) {
	if tx == nil {
		return "", ErrInvalid
	}
	if !access.Authorize(account, "user", "kick", access.ScopeBrand, brandID) {
		return "", ErrDenied
	}
	if _, err := lockMember(ctx, tx, brandID, memberID); err != nil {
		return "", err
	}
	var revoked int64
	if err := tx.QueryRow(ctx, `WITH changed AS (
		UPDATE sessions SET revoked_at=now() WHERE brand_id=$1 AND member_id=$2 AND revoked_at IS NULL RETURNING id
	) SELECT count(*) FROM changed`, brandID, memberID).Scan(&revoked); err != nil {
		return "", err
	}
	return audit.Append(ctx, tx, audit.Record{
		BrandID: brandID, ActorType: "admin", ActorID: account.ID, Action: "user.kick",
		ResourceType: "brand_member", ResourceID: memberID, Reason: reason, RequestID: requestID, IP: ip,
		Before: map[string]int64{"active_sessions": revoked}, After: map[string]int64{"active_sessions": 0},
	})
}

// ResetPassword replaces the global user's password and revokes every active
// session for that global identity across all brands.
func (s Store) ResetPassword(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, newHash, reason, requestID, ip string) (string, error) {
	if strings.TrimSpace(newHash) == "" || tx == nil {
		return "", ErrInvalid
	}
	if !access.Authorize(account, "user", "password_reset", access.ScopeBrand, brandID) {
		return "", ErrDenied
	}
	var globalUserID string
	err := tx.QueryRow(ctx, `SELECT gu.id::text FROM brand_members bm JOIN global_users gu ON gu.id=bm.global_user_id
		WHERE bm.brand_id=$1 AND bm.id=$2 FOR UPDATE OF gu`, brandID, memberID).Scan(&globalUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	// The global-user lock also serializes brand joins. A brand-local operator
	// must not set credentials that would grant access to an unowned brand.
	rows, err := tx.Query(ctx, `SELECT brand_id::text FROM brand_members WHERE global_user_id=$1`, globalUserID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var affectedBrand string
		if err = rows.Scan(&affectedBrand); err != nil {
			rows.Close()
			return "", err
		}
		if !access.Authorize(account, "user", "password_reset", access.ScopeBrand, affectedBrand) {
			rows.Close()
			return "", ErrCredentialScope
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE global_users SET password_hash=$2,updated_at=now() WHERE id=$1`, globalUserID, newHash); err != nil {
		return "", err
	}
	var revoked, brands int64
	err = tx.QueryRow(ctx, `WITH changed AS (
		UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL RETURNING brand_id
	) SELECT count(*),count(DISTINCT brand_id) FROM changed`, globalUserID).Scan(&revoked, &brands)
	if err != nil {
		return "", err
	}
	return audit.Append(ctx, tx, audit.Record{
		BrandID: brandID, ActorType: "admin", ActorID: account.ID, Action: "user.password_reset",
		ResourceType: "global_user", ResourceID: globalUserID, Reason: reason, RequestID: requestID, IP: ip,
		Before: map[string]string{"password": "redacted"},
		After:  map[string]any{"scope": "global_user_all_brands", "sessions_revoked": revoked, "brands_affected": brands},
	})
}

type memberState struct{ Status string }

func lockMember(ctx context.Context, tx pgx.Tx, brandID, memberID string) (memberState, error) {
	var state memberState
	err := tx.QueryRow(ctx, `SELECT status FROM brand_members WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brandID, memberID).Scan(&state.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberState{}, ErrNotFound
	}
	return state, err
}

func parsePermission(key string) (string, string, access.Scope, bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	scope := access.Scope(parts[2])
	if scope != access.ScopeBrand && scope != access.ScopePlatform {
		return "", "", "", false
	}
	return parts[0], parts[1], scope, true
}

func validStatus(status string) bool {
	switch status {
	case "normal", "frozen", "disabled", "expired", "cancelled":
		return true
	default:
		return false
	}
}
