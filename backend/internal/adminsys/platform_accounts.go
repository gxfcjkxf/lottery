package adminsys

import (
	"context"
	"errors"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

type platformRole struct {
	ID          string
	BrandID     string
	Code        string
	Permissions []string
}

func (s Store) ListPlatformAdmins(ctx context.Context, actor access.Account, limit, offset int) ([]AdminRecord, error) {
	if !validPage(limit, offset) {
		return nil, ErrInvalid
	}
	if !platformManagementAllowed(actor, "admin", "view") {
		return nil, ErrDenied
	}
	rows, err := s.DB.Query(ctx, `SELECT a.id::text,a.username,a.status,a.version,a.is_super_admin,
		'{}'::text[],
		COALESCE(array_agg(r.id::text ORDER BY r.code,r.id) FILTER(WHERE r.id IS NOT NULL),'{}'::text[]),
		COALESCE(array_agg(r.code ORDER BY r.code,r.id) FILTER(WHERE r.id IS NOT NULL),'{}'::text[])
		FROM admin_accounts a
		LEFT JOIN admin_account_roles ar ON ar.account_id=a.id
		LEFT JOIN roles r ON r.id=ar.role_id AND r.brand_id IS NULL
		WHERE a.is_super_admin AND NOT EXISTS(SELECT 1 FROM admin_brand_scopes s WHERE s.account_id=a.id)
		GROUP BY a.id ORDER BY a.username,a.id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminRecord, 0)
	for rows.Next() {
		var record AdminRecord
		if err := rows.Scan(&record.ID, &record.Username, &record.Status, &record.Version, &record.SuperAdmin, &record.BrandIDs, &record.RoleIDs, &record.RoleCodes); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s Store) ListPlatformRoles(ctx context.Context, actor access.Account, limit, offset int) ([]RoleRecord, error) {
	if !validPage(limit, offset) {
		return nil, ErrInvalid
	}
	if !platformManagementAllowed(actor, "role", "view") {
		return nil, ErrDenied
	}
	rows, err := s.DB.Query(ctx, `SELECT r.id::text,COALESCE(r.brand_id::text,''),r.code,r.name,r.status,r.version,r.is_bootstrap,
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE r.brand_id IS NULL
		AND NOT EXISTS(SELECT 1 FROM role_permissions invalid JOIN permissions p ON p.key=invalid.permission_key
			WHERE invalid.role_id=r.id AND split_part(p.key,'.',3)<>'platform')
		GROUP BY r.id ORDER BY r.code,r.id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RoleRecord, 0)
	for rows.Next() {
		var record RoleRecord
		if err := rows.Scan(&record.ID, &record.BrandID, &record.Code, &record.Name, &record.Status, &record.Version, &record.IsBootstrap, &record.Permissions); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s Store) CreatePlatformAdmin(ctx context.Context, tx pgx.Tx, actor access.Account, in AdminInput, passwordHash string, meta identity.Metadata) (AdminRecord, error) {
	reason, err := cleanReason(in.Reason)
	if err != nil || tx == nil || strings.TrimSpace(passwordHash) == "" {
		return AdminRecord{}, ErrInvalid
	}
	if !platformManagementAllowed(actor, "admin", "write") {
		return AdminRecord{}, ErrDenied
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if !adminNamePattern.MatchString(username) || !validAdminStatus(in.Status) {
		return AdminRecord{}, ErrInvalid
	}
	roleIDs, err := cleanRoleIDs(in.RoleIDs)
	if err != nil || len(roleIDs) == 0 {
		return AdminRecord{}, ErrInvalid
	}
	roles, err := validatePlatformRoles(ctx, tx, actor, roleIDs)
	if err != nil {
		return AdminRecord{}, err
	}
	record := AdminRecord{ID: ids.New(), Username: username, Status: in.Status, Version: 1, SuperAdmin: true, BrandIDs: []string{}, RoleIDs: make([]string, 0, len(roles)), RoleCodes: make([]string, 0, len(roles))}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,status,is_super_admin,version) VALUES($1,$2,$3,$4,true,1)`, record.ID, username, passwordHash, in.Status); isUniqueViolation(err) {
		return AdminRecord{}, ErrConflict
	} else if err != nil {
		return AdminRecord{}, err
	}
	if err = assignRoles(ctx, tx, record.ID, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	platformRoleRecords(roles, &record)
	record.AuditLogID, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "admin.create", ResourceType: "admin", ResourceID: record.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		After: map[string]any{"username": record.Username, "status": record.Status, "version": record.Version, "super_admin": true, "brand_ids": record.BrandIDs, "role_ids": record.RoleIDs}})
	return record, err
}

func (s Store) UpdatePlatformAdmin(ctx context.Context, tx pgx.Tx, actor access.Account, id string, in AdminInput, meta identity.Metadata) (AdminRecord, error) {
	reason, err := cleanReason(in.Reason)
	if err != nil || tx == nil || !validUUID(id) || in.Version < 1 {
		return AdminRecord{}, ErrInvalid
	}
	if actor.ID == id {
		return AdminRecord{}, ErrDenied
	}
	if !platformManagementAllowed(actor, "admin", "write") {
		return AdminRecord{}, ErrDenied
	}
	if !validAdminStatus(in.Status) || in.Username != "" {
		return AdminRecord{}, ErrInvalid
	}
	roleIDs, err := cleanRoleIDs(in.RoleIDs)
	if err != nil || len(roleIDs) == 0 {
		return AdminRecord{}, ErrInvalid
	}
	roles, err := validatePlatformRoles(ctx, tx, actor, roleIDs)
	if err != nil {
		return AdminRecord{}, err
	}
	record, oldRoles, err := lockPlatformAdmin(ctx, tx, id)
	if err != nil {
		return AdminRecord{}, err
	}
	if record.Version != in.Version {
		return AdminRecord{}, ErrConflict
	}
	if !actorHasPlatformRolePermissions(actor, oldRoles) {
		return AdminRecord{}, ErrDenied
	}
	oldRoleIDs := platformRoleIDs(oldRoles)
	before := platformAdminAuditSnapshot(record, oldRoleIDs)
	command, err := tx.Exec(ctx, `UPDATE admin_accounts SET status=$2,version=version+1 WHERE id=$1 AND version=$3`, id, in.Status, in.Version)
	if err != nil {
		return AdminRecord{}, err
	}
	if command.RowsAffected() != 1 {
		return AdminRecord{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM admin_account_roles ar USING roles r WHERE ar.role_id=r.id AND ar.account_id=$1 AND r.brand_id IS NULL`, id); err != nil {
		return AdminRecord{}, err
	}
	if err = assignRoles(ctx, tx, id, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE admin_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return AdminRecord{}, err
	}
	record.Status, record.Version = in.Status, record.Version+1
	record.RoleIDs, record.RoleCodes = []string{}, []string{}
	platformRoleRecords(roles, &record)
	record.AuditLogID, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "admin.update", ResourceType: "admin", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: platformAdminAuditSnapshot(record, record.RoleIDs)})
	return record, err
}

func (s Store) ResetPlatformAdminPassword(ctx context.Context, tx pgx.Tx, actor access.Account, id string, version int64, hash, reason string, meta identity.Metadata) (string, error) {
	reason, err := cleanReason(reason)
	if err != nil || tx == nil || !validUUID(id) || version < 1 || strings.TrimSpace(hash) == "" {
		return "", ErrInvalid
	}
	if actor.ID == id {
		return "", ErrDenied
	}
	if !platformManagementAllowed(actor, "admin", "write") {
		return "", ErrDenied
	}
	record, roles, err := lockPlatformAdmin(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if record.Version != version {
		return "", ErrConflict
	}
	if !actorHasPlatformRolePermissions(actor, roles) {
		return "", ErrDenied
	}
	roleIDs := platformRoleIDs(roles)
	before := platformAdminAuditSnapshot(record, roleIDs)
	command, err := tx.Exec(ctx, `UPDATE admin_accounts SET password_hash=$2,version=version+1 WHERE id=$1 AND version=$3`, id, hash, version)
	if err != nil {
		return "", err
	}
	if command.RowsAffected() != 1 {
		return "", ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE admin_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return "", err
	}
	before["password"] = "redacted"
	after := platformAdminAuditSnapshot(AdminRecord{ID: record.ID, Username: record.Username, Status: record.Status, Version: version + 1, SuperAdmin: true, BrandIDs: []string{}}, roleIDs)
	after["password"] = "redacted"
	return audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "admin.password_reset", ResourceType: "admin", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		Before: before, After: after})
}

func platformManagementAllowed(actor access.Account, resource, action string) bool {
	return actor.SuperAdmin && access.Authorize(actor, resource, action, access.ScopePlatform, "")
}

func validatePlatformRoles(ctx context.Context, tx pgx.Tx, actor access.Account, roleIDs []string) ([]platformRole, error) {
	rows, err := tx.Query(ctx, `SELECT r.id::text,COALESCE(r.brand_id::text,''),r.code,
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE r.id=ANY($1::uuid[]) AND r.brand_id IS NULL AND r.status='active'
		GROUP BY r.id ORDER BY r.code,r.id`, roleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]platformRole, 0, len(roleIDs))
	for rows.Next() {
		var role platformRole
		if err := rows.Scan(&role.ID, &role.BrandID, &role.Code, &role.Permissions); err != nil {
			return nil, err
		}
		for _, key := range role.Permissions {
			_, _, scope, ok := parsePermission(key)
			if !ok || scope != access.ScopePlatform {
				return nil, ErrInvalid
			}
		}
		if !actorHasPlatformRolePermissions(actor, []platformRole{role}) {
			return nil, ErrDenied
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(roles) != len(roleIDs) {
		return nil, ErrInvalid
	}
	return roles, nil
}

func actorHasPlatformRolePermissions(actor access.Account, roles []platformRole) bool {
	for _, role := range roles {
		for _, key := range role.Permissions {
			resource, action, scope, ok := parsePermission(key)
			if !ok || scope != access.ScopePlatform || !access.Authorize(actor, resource, action, access.ScopePlatform, "") {
				return false
			}
		}
	}
	return true
}

func lockPlatformAdmin(ctx context.Context, tx pgx.Tx, id string) (AdminRecord, []platformRole, error) {
	var record AdminRecord
	err := tx.QueryRow(ctx, `SELECT id::text,username,status,version,is_super_admin FROM admin_accounts WHERE id=$1 FOR UPDATE`, id).
		Scan(&record.ID, &record.Username, &record.Status, &record.Version, &record.SuperAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminRecord{}, nil, ErrNotFound
	}
	if err != nil {
		return AdminRecord{}, nil, err
	}
	if !record.SuperAdmin {
		return AdminRecord{}, nil, ErrDenied
	}
	brands, err := accountBrands(ctx, tx, id)
	if err != nil {
		return AdminRecord{}, nil, err
	}
	if len(brands) != 0 {
		return AdminRecord{}, nil, ErrDenied
	}
	rows, err := tx.Query(ctx, `SELECT r.id::text,COALESCE(r.brand_id::text,''),r.code,
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM admin_account_roles ar JOIN roles r ON r.id=ar.role_id LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE ar.account_id=$1 GROUP BY r.id ORDER BY r.code,r.id`, id)
	if err != nil {
		return AdminRecord{}, nil, err
	}
	roles := make([]platformRole, 0)
	for rows.Next() {
		var role platformRole
		if err := rows.Scan(&role.ID, &role.BrandID, &role.Code, &role.Permissions); err != nil {
			rows.Close()
			return AdminRecord{}, nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminRecord{}, nil, err
	}
	rows.Close()
	for _, role := range roles {
		if role.BrandID != "" {
			return AdminRecord{}, nil, ErrDenied
		}
	}
	record.SuperAdmin, record.BrandIDs = true, []string{}
	return record, roles, nil
}

func platformRoleRecords(roles []platformRole, record *AdminRecord) {
	for _, role := range roles {
		record.RoleIDs = append(record.RoleIDs, role.ID)
		record.RoleCodes = append(record.RoleCodes, role.Code)
	}
}

func platformRoleIDs(roles []platformRole) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.ID)
	}
	return result
}

func platformAdminAuditSnapshot(record AdminRecord, roleIDs []string) map[string]any {
	return map[string]any{"username": record.Username, "status": record.Status, "version": record.Version, "super_admin": true, "brand_ids": []string{}, "role_ids": roleIDs}
}
