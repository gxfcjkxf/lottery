package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"testing"
)

func securityOperator(t *testing.T, f managementHTTP) managementHTTP {
	t.Helper()
	ctx := context.Background()
	operator := f
	operator.root = ids.New()
	username := "security_" + operator.root
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,$2,password_hash,true FROM admin_accounts WHERE id=$3`, operator.root, username, f.root); err != nil {
		t.Fatal(err)
	}
	response := f.call("POST", "/api/v1/platform/auth/login", "security-login-"+operator.root, "", "", map[string]string{"identifier": username, "password": "root-test-password-2026"})
	mustStatus(t, response, 200)
	var auth identity.AdminAuthentication
	managedData(t, response, &auth)
	operator.token = auth.AccessToken
	return operator
}
