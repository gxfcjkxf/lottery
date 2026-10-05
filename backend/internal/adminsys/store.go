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
	ID           string    `json:"id"`
	GlobalUserID string    `json:"global_user_id"`
	Username     string    `json:"username"`
	Phone        string    `json:"phone"`
	DisplayName  string    `json:"display_name"`
	Notes        string    `json:"notes"`
	Status       string    `json:"status"`
	JoinedAt     time.Time `json:"joined_at"`
	BrandID      string    `json:"brand_id"`
	Tags         []string  `json:"tags"`
}

// Account loads an active admin's exact role permissions and brand scopes.
func (s Store) Account(ctx context.Context, accountID string) (access.Account, error) {
	var account access.Account
	var status string
	err := s.DB.QueryRow(ctx, `SELECT id::text,status,is_super_admin FROM admin_accounts WHERE id=$1`, accountID).
		Scan(&account.ID, &status, &account.SuperAdmin)
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

	rows, err := s.DB.Query(ctx, `SELECT r.id::text,p.key
		FROM admin_account_roles ar
		JOIN roles r ON r.id=ar.role_id
		JOIN role_permissions rp ON rp.role_id=r.id
		JOIN permissions p ON p.key=rp.permission_key
		WHERE ar.account_id=$1
		ORDER BY r.id,p.key`, accountID)
	if err != nil {
		return access.Account{}, err
	}
	roles := make(map[string]*access.Role)
	for rows.Next() {
		var roleID, key string
		if err := rows.Scan(&roleID, &key); err != nil {
			rows.Close()
			return access.Account{}, err
		}
		resource, action, scope, ok := parsePermission(key)
		if !ok {
			continue
		}
		role := roles[roleID]
		if role == nil {
			role = &access.Role{}
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

	rows, err = s.DB.Query(ctx, `SELECT brand_id::text FROM admin_brand_scopes WHERE account_id=$1 ORDER BY brand_id`, accountID)
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
func (s Store) ListMembers(ctx context.Context, account access.Account, brandID string, limit, offset int) ([]Member, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalid
	}
	platform := access.Authorize(account, "user", "view", access.ScopePlatform, "")
	brandView := brandID != "" && access.Authorize(account, "user", "view", access.ScopeBrand, brandID)
	if !platform && !brandView {
		return nil, ErrDenied
	}
	rows, err := s.DB.Query(ctx, `SELECT bm.id::text,bm.global_user_id::text,COALESCE(gu.username,''),COALESCE(gu.phone,''),
		bm.display_name,bm.notes,bm.status,bm.joined_at,bm.brand_id::text,
		CASE WHEN jsonb_typeof(bm.profile_snapshot->'tags')='array' THEN bm.profile_snapshot->'tags' ELSE '[]'::jsonb END
		FROM brand_members bm JOIN global_users gu ON gu.id=bm.global_user_id
		WHERE ($1='' OR bm.brand_id=NULLIF($1,'')::uuid)
		AND ($4::boolean OR EXISTS(SELECT 1 FROM admin_brand_scopes sc WHERE sc.account_id=$5 AND sc.brand_id=bm.brand_id))
		ORDER BY bm.joined_at DESC,bm.id LIMIT $2 OFFSET $3`, brandID, limit, offset, platform, account.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		var tags []byte
		if err := rows.Scan(&member.ID, &member.GlobalUserID, &member.Username, &member.Phone, &member.DisplayName,
			&member.Notes, &member.Status, &member.JoinedAt, &member.BrandID, &tags); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tags, &member.Tags); err != nil {
			return nil, fmt.Errorf("decode member tags: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

// ChangeMember changes only a membership in the specified brand. Snapshots
// contain status alone; identity, credentials, notes, and contact details are
// omitted from the audit record.
func (s Store) ChangeMember(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, status, notes, reason, requestID, ip string) (string, error) {
	if !validStatus(status) || strings.TrimSpace(reason) == "" || tx == nil {
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

// Kick revokes only sessions for the selected member in the selected brand.
func (s Store) Kick(ctx context.Context, tx pgx.Tx, account access.Account, brandID, memberID, reason, requestID, ip string) (string, error) {
	if strings.TrimSpace(reason) == "" || tx == nil {
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
	if strings.TrimSpace(newHash) == "" || strings.TrimSpace(reason) == "" || tx == nil {
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
