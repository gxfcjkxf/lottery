package adminsys

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrConflict reports a stale version or a unique-value collision.
var ErrConflict = errors.New("administrative resource conflict")

type RoleRecord struct {
	ID          string   `json:"id"`
	BrandID     string   `json:"brand_id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Version     int64    `json:"version"`
	IsBootstrap bool     `json:"is_bootstrap"`
	Permissions []string `json:"permissions"`
	AuditLogID  string   `json:"audit_log_id,omitempty"`
}

type AdminRecord struct {
	ID         string   `json:"id"`
	Username   string   `json:"username"`
	Status     string   `json:"status"`
	Version    int64    `json:"version"`
	SuperAdmin bool     `json:"super_admin"`
	BrandIDs   []string `json:"brand_ids"`
	RoleIDs    []string `json:"role_ids"`
	RoleCodes  []string `json:"role_codes"`
	AuditLogID string   `json:"audit_log_id,omitempty"`
}

type RoleInput struct {
	Version     int64    `json:"version"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
	Reason      string   `json:"reason"`
}

type AdminInput struct {
	Username string   `json:"username"`
	Version  int64    `json:"version"`
	RoleIDs  []string `json:"role_ids"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason"`
}

var (
	roleCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{2,47}$`)
	adminNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)
	uuidPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func (s Store) ListRoles(ctx context.Context, actor access.Account, brand string, limit, offset int) ([]RoleRecord, error) {
	if !validUUID(brand) || !validPage(limit, offset) {
		return nil, ErrInvalid
	}
	if !ManagementAllowed(actor, "role", "view", brand) {
		return nil, ErrDenied
	}
	rows, err := s.DB.Query(ctx, `SELECT r.id::text,r.brand_id::text,r.code,r.name,r.status,r.version,r.is_bootstrap,
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE r.brand_id=$1 GROUP BY r.id ORDER BY r.code,r.id LIMIT $2 OFFSET $3`, brand, limit, offset)
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

func (s Store) ListAdmins(ctx context.Context, actor access.Account, brand string, limit, offset int) ([]AdminRecord, error) {
	if !validUUID(brand) || !validPage(limit, offset) {
		return nil, ErrInvalid
	}
	if !ManagementAllowed(actor, "admin", "view", brand) {
		return nil, ErrDenied
	}
	platform := ManagementAllowed(actor, "admin", "view", "")
	rows, err := s.DB.Query(ctx, `SELECT a.id::text,a.username,a.status,a.version,a.is_super_admin,
		ARRAY(SELECT s.brand_id::text FROM admin_brand_scopes s WHERE s.account_id=a.id ORDER BY s.brand_id),
		COALESCE(array_agg(r.id::text ORDER BY r.code,r.id) FILTER(WHERE r.id IS NOT NULL AND r.brand_id=$1),'{}'::text[]),
		COALESCE(array_agg(r.code ORDER BY r.code,r.id) FILTER(WHERE r.id IS NOT NULL AND r.brand_id=$1),'{}'::text[])
		FROM admin_accounts a JOIN admin_brand_scopes target ON target.account_id=a.id AND target.brand_id=$1
		LEFT JOIN admin_account_roles ar ON ar.account_id=a.id LEFT JOIN roles r ON r.id=ar.role_id
		WHERE $2::boolean OR (NOT a.is_super_admin AND (SELECT count(*) FROM admin_brand_scopes s WHERE s.account_id=a.id)=1)
		GROUP BY a.id ORDER BY a.username,a.id LIMIT $3 OFFSET $4`, brand, platform, limit, offset)
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

func (s Store) ListPermissions(ctx context.Context, actor access.Account, brand string) ([]string, error) {
	if !validUUID(brand) {
		return nil, ErrInvalid
	}
	if !ManagementAllowed(actor, "role", "view", brand) {
		return nil, ErrDenied
	}
	rows, err := s.DB.Query(ctx, `SELECT key FROM permissions WHERE split_part(key,'.',3)='brand' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		permissions = append(permissions, key)
	}
	return permissions, rows.Err()
}

func (s Store) WriteRole(ctx context.Context, tx pgx.Tx, actor access.Account, brand, id string, in RoleInput, meta identity.Metadata) (RoleRecord, error) {
	reason, err := cleanReason(in.Reason)
	if err != nil || tx == nil || !validUUID(brand) || (id != "" && !validUUID(id)) {
		return RoleRecord{}, ErrInvalid
	}
	if !ManagementAllowed(actor, "role", "write", brand) {
		return RoleRecord{}, ErrDenied
	}
	if id == "" && in.Status == "" {
		in.Status = "active"
	}
	if !validRoleFields(in.Name, in.Status) {
		return RoleRecord{}, ErrInvalid
	}
	permissions, err := cleanPermissions(in.Permissions)
	if err != nil {
		return RoleRecord{}, err
	}
	platformGrant := ManagementAllowed(actor, "role", "write", "")
	if !platformGrant && !actorHasPermissions(actor, permissions, brand) {
		return RoleRecord{}, ErrDenied
	}
	for _, key := range permissions {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM permissions WHERE key=$1 AND split_part(key,'.',3)='brand')`, key).Scan(&exists); err != nil {
			return RoleRecord{}, err
		}
		if !exists {
			return RoleRecord{}, ErrInvalid
		}
	}

	created := id == ""
	var record RoleRecord
	var before map[string]any
	if created {
		code := strings.TrimSpace(in.Code)
		if !roleCodePattern.MatchString(code) {
			return RoleRecord{}, ErrInvalid
		}
		record = RoleRecord{ID: ids.New(), BrandID: brand, Code: code, Name: in.Name, Status: in.Status, Version: 1, Permissions: permissions}
		_, err = tx.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,status,version,is_bootstrap) VALUES($1,$2,$3,$4,$5,1,false)`, record.ID, brand, code, in.Name, in.Status)
		if isUniqueViolation(err) {
			return RoleRecord{}, ErrConflict
		}
		if err != nil {
			return RoleRecord{}, err
		}
		if err := replaceRolePermissions(ctx, tx, record.ID, permissions); err != nil {
			return RoleRecord{}, err
		}
	} else {
		if in.Version < 1 {
			return RoleRecord{}, ErrInvalid
		}
		err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,code,name,status,version,is_bootstrap FROM roles WHERE id=$1 AND brand_id=$2 FOR UPDATE`, id, brand).
			Scan(&record.ID, &record.BrandID, &record.Code, &record.Name, &record.Status, &record.Version, &record.IsBootstrap)
		if errors.Is(err, pgx.ErrNoRows) {
			return RoleRecord{}, ErrNotFound
		}
		if err != nil {
			return RoleRecord{}, err
		}
		if record.IsBootstrap {
			return RoleRecord{}, ErrDenied
		}
		if record.Version != in.Version {
			return RoleRecord{}, ErrConflict
		}
		beforePermissions, err := loadRolePermissions(ctx, tx, id)
		if err != nil {
			return RoleRecord{}, err
		}
		if !platformGrant && !actorHasPermissions(actor, beforePermissions, brand) {
			return RoleRecord{}, ErrDenied
		}
		var actorHasRole bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_account_roles WHERE account_id=$1 AND role_id=$2)`, actor.ID, id).Scan(&actorHasRole); err != nil {
			return RoleRecord{}, err
		}
		if actorHasRole {
			return RoleRecord{}, ErrDenied
		}
		if in.Code != "" && in.Code != record.Code {
			return RoleRecord{}, ErrInvalid
		}
		before = map[string]any{"code": record.Code, "name": record.Name, "status": record.Status, "version": record.Version, "permissions": beforePermissions}
		command, e := tx.Exec(ctx, `UPDATE roles SET name=$3,status=$4,version=version+1 WHERE id=$1 AND brand_id=$2 AND version=$5`, id, brand, in.Name, in.Status, in.Version)
		if e != nil {
			return RoleRecord{}, e
		}
		if command.RowsAffected() != 1 {
			return RoleRecord{}, ErrConflict
		}
		if err := replaceRolePermissions(ctx, tx, id, permissions); err != nil {
			return RoleRecord{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE admin_accounts a SET version=version+1 FROM admin_account_roles ar WHERE ar.role_id=$1 AND ar.account_id=a.id`, id); err != nil {
			return RoleRecord{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE revoked_at IS NULL AND admin_id IN (SELECT account_id FROM admin_account_roles WHERE role_id=$1)`, id); err != nil {
			return RoleRecord{}, err
		}
		record.Name, record.Status, record.Version = in.Name, in.Status, record.Version+1
		record.Permissions = permissions
	}
	action := "role.create"
	if !created {
		action = "role.update"
	}
	change, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: action, ResourceType: "role", ResourceID: record.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"code": record.Code, "name": record.Name, "status": record.Status, "version": record.Version, "permissions": permissions}})
	if err != nil {
		return RoleRecord{}, err
	}
	record.AuditLogID = change
	return record, nil
}

func (s Store) CreateAdmin(ctx context.Context, tx pgx.Tx, actor access.Account, brand string, in AdminInput, passwordHash string, meta identity.Metadata) (AdminRecord, error) {
	reason, err := cleanReason(in.Reason)
	if err != nil || tx == nil || !validUUID(brand) || strings.TrimSpace(passwordHash) == "" {
		return AdminRecord{}, ErrInvalid
	}
	if !ManagementAllowed(actor, "admin", "write", brand) {
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
	if err := validateBrandRoles(ctx, tx, brand, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	if err := validateRoleDelegation(ctx, tx, actor, brand, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	record := AdminRecord{ID: ids.New(), Username: username, Status: in.Status, Version: 1, BrandIDs: []string{brand}, RoleIDs: make([]string, 0, len(roleIDs)), RoleCodes: make([]string, 0, len(roleIDs))}
	_, err = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,status,is_super_admin,version) VALUES($1,$2,$3,$4,false,1)`, record.ID, username, passwordHash, in.Status)
	if isUniqueViolation(err) {
		return AdminRecord{}, ErrConflict
	}
	if err != nil {
		return AdminRecord{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, record.ID, brand); err != nil {
		return AdminRecord{}, err
	}
	if err = assignRoles(ctx, tx, record.ID, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	if err = loadSelectedRoles(ctx, tx, brand, roleIDs, &record); err != nil {
		return AdminRecord{}, err
	}
	record.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "admin.create", ResourceType: "admin", ResourceID: record.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"username": record.Username, "status": record.Status, "version": record.Version, "super_admin": false, "brand_ids": record.BrandIDs, "role_ids": record.RoleIDs}})
	return record, err
}

func (s Store) UpdateAdmin(ctx context.Context, tx pgx.Tx, actor access.Account, brand, id string, in AdminInput, meta identity.Metadata) (AdminRecord, error) {
	reason, err := cleanReason(in.Reason)
	if err != nil || tx == nil || !validUUID(brand) || !validUUID(id) || in.Version < 1 {
		return AdminRecord{}, ErrInvalid
	}
	if actor.ID == id {
		return AdminRecord{}, ErrDenied
	}
	if !ManagementAllowed(actor, "admin", "write", brand) {
		return AdminRecord{}, ErrDenied
	}
	if !validAdminStatus(in.Status) {
		return AdminRecord{}, ErrInvalid
	}
	if in.Username != "" {
		return AdminRecord{}, ErrInvalid
	}
	roleIDs, err := cleanRoleIDs(in.RoleIDs)
	if err != nil || len(roleIDs) == 0 {
		return AdminRecord{}, ErrInvalid
	}
	if err = validateBrandRoles(ctx, tx, brand, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	var record AdminRecord
	err = tx.QueryRow(ctx, `SELECT id::text,username,status,version,is_super_admin FROM admin_accounts WHERE id=$1 FOR UPDATE`, id).
		Scan(&record.ID, &record.Username, &record.Status, &record.Version, &record.SuperAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminRecord{}, ErrNotFound
	}
	if err != nil {
		return AdminRecord{}, err
	}
	if record.SuperAdmin {
		return AdminRecord{}, ErrDenied
	}
	if record.Version != in.Version {
		return AdminRecord{}, ErrConflict
	}
	record.BrandIDs, err = accountBrands(ctx, tx, id)
	if err != nil {
		return AdminRecord{}, err
	}
	platform := ManagementAllowed(actor, "admin", "write", "")
	if !containsString(record.BrandIDs, brand) {
		return AdminRecord{}, ErrNotFound
	}
	if !platform && len(record.BrandIDs) != 1 {
		return AdminRecord{}, ErrDenied
	}
	oldRoles, err := loadAccountRoles(ctx, tx, id)
	if err != nil {
		return AdminRecord{}, err
	}
	if err := validateTargetRoles(actor, brand, platform, oldRoles); err != nil {
		return AdminRecord{}, err
	}
	if err := validateRoleDelegation(ctx, tx, actor, brand, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	oldRoleIDs := roleIDsFrom(oldRoles)
	before := map[string]any{"username": record.Username, "status": record.Status, "version": record.Version, "brand_ids": record.BrandIDs, "role_ids": oldRoleIDs}
	command, err := tx.Exec(ctx, `UPDATE admin_accounts SET status=$2,version=version+1 WHERE id=$1 AND version=$3`, id, in.Status, in.Version)
	if err != nil {
		return AdminRecord{}, err
	}
	if command.RowsAffected() != 1 {
		return AdminRecord{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM admin_account_roles ar USING roles r WHERE ar.role_id=r.id AND ar.account_id=$1 AND r.brand_id=$2`, id, brand); err != nil {
		return AdminRecord{}, err
	}
	if err = assignRoles(ctx, tx, id, roleIDs); err != nil {
		return AdminRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE admin_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return AdminRecord{}, err
	}
	record.Status, record.Version = in.Status, record.Version+1
	if err = loadSelectedRoles(ctx, tx, brand, roleIDs, &record); err != nil {
		return AdminRecord{}, err
	}
	newRoles, err := loadAccountRoles(ctx, tx, id)
	if err != nil {
		return AdminRecord{}, err
	}
	record.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "admin.update", ResourceType: "admin", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: map[string]any{"username": record.Username, "status": record.Status, "version": record.Version, "brand_ids": record.BrandIDs, "role_ids": roleIDsFrom(newRoles)}})
	return record, err
}

func (s Store) ResetAdminPassword(ctx context.Context, tx pgx.Tx, actor access.Account, brand, id string, version int64, hash, reason string, meta identity.Metadata) (string, error) {
	reason, err := cleanReason(reason)
	if err != nil || tx == nil || !validUUID(brand) || !validUUID(id) || version < 1 || strings.TrimSpace(hash) == "" {
		return "", ErrInvalid
	}
	if actor.ID == id {
		return "", ErrDenied
	}
	if !ManagementAllowed(actor, "admin", "write", brand) {
		return "", ErrDenied
	}
	var username, status string
	var super bool
	var current int64
	err = tx.QueryRow(ctx, `SELECT username,status,is_super_admin,version FROM admin_accounts WHERE id=$1 FOR UPDATE`, id).Scan(&username, &status, &super, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if super {
		return "", ErrDenied
	}
	brands, err := accountBrands(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !containsString(brands, brand) {
		return "", ErrNotFound
	}
	platform := ManagementAllowed(actor, "admin", "write", "")
	if !platform && len(brands) != 1 {
		return "", ErrDenied
	}
	roles, err := loadAccountRoles(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if err := validateTargetRoles(actor, brand, platform, roles); err != nil {
		return "", err
	}
	if current != version {
		return "", ErrConflict
	}
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
	return audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "admin.password_reset", ResourceType: "admin", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"password": "redacted", "username": username, "status": status, "version": current, "brand_ids": brands, "role_ids": roleIDsFrom(roles)}, After: map[string]any{"password": "redacted", "username": username, "status": status, "version": current + 1, "brand_ids": brands, "role_ids": roleIDsFrom(roles)}})
}

func validPage(limit, offset int) bool { return limit >= 1 && limit <= 100 && offset >= 0 }

func validRoleFields(name, status string) bool {
	return utf8.ValidString(name) && len(name) >= 1 && len(name) <= 120 && strings.TrimSpace(name) == name && (status == "active" || status == "disabled")
}

func validAdminStatus(status string) bool { return status == "active" || status == "disabled" }

func cleanReason(reason string) (string, error) {
	if !utf8.ValidString(reason) {
		return "", ErrInvalid
	}
	reason = strings.TrimSpace(reason)
	if len(reason) < 1 || len(reason) > 500 {
		return "", ErrInvalid
	}
	return reason, nil
}

func cleanPermissions(values []string) ([]string, error) {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		parts := strings.Split(value, ".")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != "brand" {
			return nil, ErrInvalid
		}
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func cleanRoleIDs(values []string) ([]string, error) {
	if len(values) > 100 {
		return nil, ErrInvalid
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validUUID(value) {
			return nil, ErrInvalid
		}
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func actorHasPermissions(actor access.Account, keys []string, brand string) bool {
	for _, key := range keys {
		parts := strings.Split(key, ".")
		if len(parts) != 3 || !access.Authorize(actor, parts[0], parts[1], access.ScopeBrand, brand) {
			return false
		}
	}
	return true
}

func validUUID(value string) bool { return uuidPattern.MatchString(value) }

func loadRolePermissions(ctx context.Context, tx pgx.Tx, roleID string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT permission_key FROM role_permissions WHERE role_id=$1 ORDER BY permission_key`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func validateRoleDelegation(ctx context.Context, tx pgx.Tx, actor access.Account, brand string, roleIDs []string) error {
	if ManagementAllowed(actor, "admin", "write", "") {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT r.id::text,COALESCE(r.brand_id::text,''),
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id WHERE r.id=ANY($1::uuid[]) GROUP BY r.id ORDER BY r.id`, roleIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, roleBrand string
		var permissions []string
		if err := rows.Scan(&id, &roleBrand, &permissions); err != nil {
			return err
		}
		count++
		if roleBrand != brand || !actorHasPermissions(actor, permissions, brand) {
			return ErrDenied
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(roleIDs) {
		return ErrInvalid
	}
	return nil
}

type assignedRole struct {
	ID          string
	BrandID     string
	Permissions []string
}

func loadAccountRoles(ctx context.Context, tx pgx.Tx, accountID string) ([]assignedRole, error) {
	rows, err := tx.Query(ctx, `SELECT r.id::text,COALESCE(r.brand_id::text,''),
		COALESCE(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER(WHERE rp.permission_key IS NOT NULL),'{}'::text[])
		FROM admin_account_roles ar JOIN roles r ON r.id=ar.role_id LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE ar.account_id=$1 GROUP BY r.id ORDER BY r.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]assignedRole, 0)
	for rows.Next() {
		var role assignedRole
		if err := rows.Scan(&role.ID, &role.BrandID, &role.Permissions); err != nil {
			return nil, err
		}
		result = append(result, role)
	}
	return result, rows.Err()
}

func validateTargetRoles(actor access.Account, brand string, platform bool, roles []assignedRole) error {
	if platform {
		return nil
	}
	for _, role := range roles {
		if role.BrandID != brand || !actorHasPermissions(actor, role.Permissions, brand) {
			return ErrDenied
		}
	}
	return nil
}

func roleIDsFrom(roles []assignedRole) []string {
	ids := make([]string, 0, len(roles))
	for _, role := range roles {
		ids = append(ids, role.ID)
	}
	return ids
}

func replaceRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, permissions []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, roleID); err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, roleID, permission); err != nil {
			return err
		}
	}
	return nil
}

func validateBrandRoles(ctx context.Context, tx pgx.Tx, brand string, roleIDs []string) error {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(DISTINCT id) FROM roles WHERE brand_id=$1 AND status='active' AND id=ANY($2::uuid[])`, brand, roleIDs).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(roleIDs) {
		return ErrInvalid
	}
	return nil
}

func assignRoles(ctx context.Context, tx pgx.Tx, accountID string, roleIDs []string) error {
	for _, roleID := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, accountID, roleID); err != nil {
			return err
		}
	}
	return nil
}

func loadSelectedRoles(ctx context.Context, tx pgx.Tx, brand string, roleIDs []string, record *AdminRecord) error {
	rows, err := tx.Query(ctx, `SELECT id::text,code FROM roles WHERE brand_id=$1 AND id=ANY($2::uuid[]) ORDER BY code,id`, brand, roleIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	record.RoleIDs = make([]string, 0, len(roleIDs))
	record.RoleCodes = make([]string, 0, len(roleIDs))
	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			return err
		}
		record.RoleIDs = append(record.RoleIDs, id)
		record.RoleCodes = append(record.RoleCodes, code)
	}
	return rows.Err()
}

func accountBrands(ctx context.Context, tx pgx.Tx, accountID string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT brand_id::text FROM admin_brand_scopes WHERE account_id=$1 ORDER BY brand_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	brands := make([]string, 0)
	for rows.Next() {
		var brand string
		if err := rows.Scan(&brand); err != nil {
			return nil, err
		}
		brands = append(brands, brand)
	}
	return brands, rows.Err()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
