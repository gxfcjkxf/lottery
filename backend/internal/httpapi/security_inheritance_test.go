package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"strconv"
	"testing"
)

func TestSecurityDefaultsInheritanceAndThreeLoginSurfaces(t *testing.T) {
	f := managedFixture(t)
	op := securityOperator(t, f)
	grantPlatformPermission(t, op, "auth_config.view.platform", "auth_config.write.platform")
	ctx := context.Background()
	const authPath = "/api/v1/platform/auth-settings"
	var defaults identity.AuthSettingsRecord
	r := op.call("GET", authPath, "", op.token, "", nil)
	mustStatus(t, r, 200)
	managedData(t, r, &defaults)
	in := identity.AuthSettingsInput{Version: defaults.Version, CaptchaEnabled: true}
	saved := op.call("PATCH", authPath, "defaults-captcha-on", op.token, "", in)
	mustStatus(t, saved, 200)
	mustStatus(t, op.call("PATCH", authPath, "defaults-captcha-on", op.token, "", in), 200)
	mustStatus(t, op.call("PATCH", authPath, "defaults-stale", op.token, "", in), 409)
	mustStatus(t, f.call("PATCH", authPath, "defaults-staff-denied", f.token, "", in), 403)
	// Explicit brand override is isolated from the inherited brand.
	var brand identity.AuthSettingsRecord
	r = op.call("GET", authPath, "", op.token, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &brand)
	mustStatus(t, op.call("PATCH", authPath, "brand-captcha-off", op.token, managedBrand, identity.AuthSettingsInput{Version: brand.Version}), 200)
	cfg, err := fIdentitySettings(f, ctx, managedBrand)
	if err != nil || cfg.CaptchaEnabled {
		t.Fatal(cfg, err)
	}
	r = op.call("GET", authPath, "", op.token, managedBrand, nil)
	managedData(t, r, &brand)
	mustStatus(t, op.call("PATCH", authPath, "brand-inherit", op.token, managedBrand, identity.AuthSettingsInput{Version: brand.Version, InheritPlatform: true}), 200)
	for i, path := range []string{"/api/v1/auth/login", "/api/v1/admin/auth/login", "/api/v1/agent/login"} {
		body := map[string]string{"identifier": "missing_account", "password": "invalid123"}
		if i == 2 {
			body = map[string]string{"username": "missing_account", "password": "invalid123"}
		}
		mustStatus(t, f.call("POST", path, "missing-captcha-"+strconv.Itoa(i), "", managedBrand, body), 400)
		// A valid captcha reaches credential validation and is consumed even on bad credentials.
		challenge := f.call("GET", "/api/v1/auth/challenge", "", "", managedBrand, nil)
		mustStatus(t, challenge, 200)
		var c identity.Challenge
		managedData(t, challenge, &c)
		var hash string
		if err := f.pool.QueryRow(ctx, "SELECT proof_hash FROM auth_challenges WHERE id=$1", c.ID).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		engine, _ := mutation.New(f.pool, make([]byte, 32))
		answer := ""
		for x := 0; x <= 300; x++ {
			if engine.Fingerprint(c.ID+":"+strconv.Itoa(x)) == hash {
				answer = strconv.Itoa(x)
				break
			}
		}
		if answer == "" {
			t.Fatal("missing puzzle target")
		}
		body["captcha_id"] = c.ID
		body["captcha_answer"] = answer
		mustStatus(t, f.call("POST", path, "valid-captcha-"+strconv.Itoa(i), "", managedBrand, body), 401)
		mustStatus(t, f.call("POST", path, "reused-captcha-"+strconv.Itoa(i), "", managedBrand, body), 400)
	}
	r = op.call("GET", authPath, "", op.token, "", nil)
	managedData(t, r, &defaults)
	mustStatus(t, op.call("PATCH", authPath, "defaults-captcha-off", op.token, "", identity.AuthSettingsInput{Version: defaults.Version}), 200)
	cfg, err = fIdentitySettings(f, ctx, managedBrand)
	if err != nil || cfg.CaptchaEnabled {
		t.Fatal("inherited auth did not update", err)
	}
}
func fIdentitySettings(f managementHTTP, ctx context.Context, brand string) (identity.Settings, error) {
	s, e := identity.New(f.pool)
	if e != nil {
		return identity.Settings{}, e
	}
	return s.Settings(ctx, brand)
}
