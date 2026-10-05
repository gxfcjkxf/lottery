// Package access provides pure role and scope authorization checks.
package access

import "sort"

type Scope string

const (
	ScopePlatform Scope = "platform"
	ScopeBrand    Scope = "brand"
)

type AccountType string

const (
	AccountUser  AccountType = "user"
	AccountAdmin AccountType = "admin"
)

// Permission is an exact resource.action.scope grant. No field is interpreted
// as a wildcard; callers must request the same values that were granted.
type Permission struct {
	Resource string
	Action   string
	Scope    Scope
}

// Role contains grants only; roles have no implicit hierarchy.
type Role struct {
	Permissions []Permission
}

// Account is the authorization context for a user or administrative account.
// BrandIDs are the brands in which this account has been granted membership.
type Account struct {
	ID         string
	Type       AccountType
	SuperAdmin bool
	Roles      []Role
	BrandIDs   []string
}

// UnionPermissions returns the distinct explicit permissions across roles in
// deterministic lexicographic order.
func UnionPermissions(roles ...Role) []Permission {
	set := make(map[Permission]struct{})
	for _, role := range roles {
		for _, permission := range role.Permissions {
			set[permission] = struct{}{}
		}
	}
	permissions := make([]Permission, 0, len(set))
	for permission := range set {
		permissions = append(permissions, permission)
	}
	sort.Slice(permissions, func(i, j int) bool {
		if permissions[i].Resource != permissions[j].Resource {
			return permissions[i].Resource < permissions[j].Resource
		}
		if permissions[i].Action != permissions[j].Action {
			return permissions[i].Action < permissions[j].Action
		}
		return permissions[i].Scope < permissions[j].Scope
	})
	return permissions
}

// Authorize permits only administrative accounts with an exact permission.
// Brand scope also requires membership in the requested brand. Platform scope
// does not inherit brand grants, and brand grants do not inherit platform scope.
func Authorize(account Account, resource, action string, scope Scope, brandID string) bool {
	if account.Type != AccountAdmin || resource == "" || action == "" {
		return false
	}
	if scope != ScopePlatform && scope != ScopeBrand {
		return false
	}
	if scope == ScopeBrand && (brandID == "" || !contains(account.BrandIDs, brandID)) {
		return false
	}
	// Super administrators can inspect user records but cannot mutate users or
	// kick their sessions, even if a role accidentally contains such a grant.
	if account.SuperAdmin && resource == "user" && action != "view" {
		return false
	}
	for _, role := range account.Roles {
		for _, permission := range role.Permissions {
			if permission.Resource == resource && permission.Action == action && permission.Scope == scope {
				return true
			}
		}
	}
	return false
}

// CanReviewRule applies the separation-of-duties rule to brand rule reviews.
func CanReviewRule(reviewer Account, brandID, creatorID string) bool {
	return !reviewer.SuperAdmin && reviewer.ID != "" && creatorID != "" && reviewer.ID != creatorID &&
		Authorize(reviewer, "rule", "review", ScopeBrand, brandID)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
