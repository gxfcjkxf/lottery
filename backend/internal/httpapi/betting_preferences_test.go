package httpapi

import (
	"context"
	"reflect"
	"testing"
)

func TestQuickAmountsValidation(t *testing.T) {
	for _, v := range [][]string{nil, {"1000000"}, {"0"}, {"-1"}, {"1.5"}, {"01"}, {"9223372036854775808"}, {"1", "2", "3", "4", "5", "6"}} {
		if _, ok := parseQuickAmounts(v); ok {
			t.Fatalf("accepted %v", v)
		}
	}
	for _, v := range [][]string{{}, {"1", "50", "999999"}} {
		if _, ok := parseQuickAmounts(v); !ok {
			t.Fatalf("rejected %v", v)
		}
	}
}
func TestMemberQuickAmountsHTTP(t *testing.T) {
	f := pointsFixture(t)
	path := "/api/v1/b/aurora/me/betting-preferences"
	read := func(token string) quickAmountsOutput {
		t.Helper()
		r := f.call("GET", path, "", token, managedBrand, nil)
		mustStatus(t, r, 200)
		var out quickAmountsOutput
		managedData(t, r, &out)
		return out
	}
	if out := read(f.userToken); out.BrandID != managedBrand || out.MemberID != f.memberID || len(out.Amounts) != 0 {
		t.Fatalf("initial %+v", out)
	}
	values := []string{"5", "20", "50", "100", "999999"}
	mustStatus(t, f.call("PUT", path, "", f.userToken, managedBrand, map[string]any{"quick_amounts": values}), 200)
	// A fresh login represents another device; values live on the member, not the session.
	login := f.call("POST", "/api/v1/auth/login", "preferences-second-device", "", managedBrand, map[string]string{"identifier": "points_member", "password": "points-member-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, login, 200)
	var auth struct {
		AccessToken string `json:"access_token"`
	}
	managedData(t, login, &auth)
	if out := read(auth.AccessToken); !reflect.DeepEqual(out.Amounts, values) {
		t.Fatalf("second device %+v", out)
	}
	mustStatus(t, f.call("GET", path, "", f.userToken, managedBrand, nil), 401)
	for _, body := range []any{map[string]any{"quick_amounts": []string{"1", "2", "3", "4", "5", "6"}}, map[string]any{"quick_amounts": []string{"0"}}, map[string]any{"quick_amounts": nil}, map[string]any{"quick_amounts": []string{}, "member_id": f.memberID}} {
		mustStatus(t, f.call("PUT", path, "", auth.AccessToken, managedBrand, body), 400)
	}
	if out := read(auth.AccessToken); !reflect.DeepEqual(out.Amounts, values) {
		t.Fatal("invalid write changed preferences")
	}
	mustStatus(t, f.call("PUT", path, "", "", managedBrand, map[string]any{"quick_amounts": []string{}}), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/me/betting-preferences", "", auth.AccessToken, "", nil), 401)
	mustStatus(t, f.call("PUT", "/api/v1/b/harbor/me/betting-preferences", "", auth.AccessToken, "", map[string]any{"quick_amounts": []string{}}), 401)
	created := f.call("POST", "/api/v1/admin/users", "preferences-other-member", f.token, managedBrand, map[string]string{"username": "preferences_other", "password": "other-password-2026", "display_name": "Other member"})
	mustStatus(t, created, 201)
	otherLogin := f.call("POST", "/api/v1/auth/login", "preferences-other-login", "", managedBrand, map[string]string{"identifier": "preferences_other", "password": "other-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, otherLogin, 200)
	var other struct {
		AccessToken string `json:"access_token"`
	}
	managedData(t, otherLogin, &other)
	if out := read(other.AccessToken); len(out.Amounts) != 0 || out.MemberID == f.memberID {
		t.Fatal("preferences leaked to another member")
	}
	mustStatus(t, f.call("PUT", path, "", other.AccessToken, managedBrand, map[string]any{"quick_amounts": []string{"25"}}), 200)
	if out := read(auth.AccessToken); !reflect.DeepEqual(out.Amounts, values) {
		t.Fatal("other member changed preferences")
	}
	mustStatus(t, f.call("GET", path+"?member_id="+f.memberID, "", auth.AccessToken, managedBrand, nil), 400)
	mustStatus(t, f.call("PUT", "/api/v1/me/betting-preferences", "", auth.AccessToken, managedBrand, map[string]any{"quick_amounts": []string{}}), 200)
	if len(read(auth.AccessToken).Amounts) != 0 {
		t.Fatal("clear not persisted")
	}
	// Database rules also protect the five positive-integer limit.
	if _, err := f.pool.Exec(context.Background(), `UPDATE brand_members SET betting_quick_amounts=ARRAY[0]::bigint[] WHERE id=$1`, f.memberID); err == nil {
		t.Fatal("database accepted zero")
	}
}
