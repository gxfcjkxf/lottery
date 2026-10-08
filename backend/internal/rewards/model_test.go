package rewards

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

const (
	testBrand  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testBrand2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testMember = "abcdefab-cdef-4abc-8def-abcdefabcdef"
)

func TestGrantInputJSONExactAmountAndValidation(t *testing.T) {
	const maxInt64 = "9223372036854775807"
	raw := `{"member_id":"` + testMember + `","points":"` + maxInt64 + `","reason":"manual reward"}`
	var in GrantInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatalf("decode max amount: %v", err)
	}
	if int64(in.Points) != int64(^uint64(0)>>1) {
		t.Fatalf("amount lost precision: got %d", in.Points)
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"points":"`+maxInt64+`"`) {
		t.Fatalf("amount was not encoded as its exact decimal string: %s", encoded)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("valid grant rejected: %v", err)
	}

	for _, amount := range []string{"0", "-1", "01", "+1", "1.0", "1e2", "9223372036854775808"} {
		t.Run("amount_"+amount, func(t *testing.T) {
			body := `{"member_id":"` + testMember + `","points":"` + amount + `","reason":"ok"}`
			if json.Unmarshal([]byte(body), &GrantInput{}) == nil {
				t.Fatalf("accepted invalid amount %q", amount)
			}
		})
	}
	if json.Unmarshal([]byte(`{"member_id":"`+testMember+`","points":1,"reason":"ok"}`), &GrantInput{}) == nil {
		t.Fatal("accepted numeric JSON amount instead of exact string")
	}
}

func TestActionInputVersionSafeIntegerBounds(t *testing.T) {
	valid := fmt.Sprintf(`{"version":%d,"reason":"retry after review"}`, MaxVersion)
	var in ActionInput
	if err := json.Unmarshal([]byte(valid), &in); err != nil {
		t.Fatalf("decode MaxVersion: %v", err)
	}
	if in.Version != MaxVersion || in.Validate() != nil {
		t.Fatalf("MaxVersion not preserved: %+v", in)
	}
	for _, raw := range []string{
		`{"version":0,"reason":"ok"}`,
		`{"version":-1,"reason":"ok"}`,
		`{"version":9007199254740992,"reason":"ok"}`,
		`{"version":1.0,"reason":"ok"}`,
		`{"version":1e0,"reason":"ok"}`,
		`{"version":"1","reason":"ok"}`,
	} {
		if json.Unmarshal([]byte(raw), &ActionInput{}) == nil {
			t.Errorf("accepted invalid version body: %s", raw)
		}
	}
}

func TestInputJSONRequiresClosedCompleteNonNullObjects(t *testing.T) {
	grantValid := `{"member_id":"` + testMember + `","points":"1","reason":"ok"}`
	actionValid := `{"version":1,"reason":"ok"}`
	grantBad := []string{
		`null`, `[]`, `{}`, `{"member_id":"` + testMember + `","points":"1"}`,
		strings.TrimSuffix(grantValid, "}") + `,"extra":true}`,
		strings.TrimSuffix(grantValid, "}") + `,"member_id":"` + testMember + `"}`,
		`{"member_id":null,"points":"1","reason":"ok"}`,
		`{"member_id":"` + testMember + `","points":null,"reason":"ok"}`,
		`{"member_id":"` + testMember + `","points":"1","reason":null}`,
		grantValid + ` {}`,
	}
	for i, raw := range grantBad {
		t.Run(fmt.Sprintf("grant_%d", i), func(t *testing.T) {
			if json.Unmarshal([]byte(raw), &GrantInput{}) == nil {
				t.Fatalf("accepted body %s", raw)
			}
		})
	}
	actionBad := []string{
		`null`, `[]`, `{}`, `{"version":1}`, `{"version":1,"reason":"ok","extra":0}`,
		`{"version":1,"version":1,"reason":"ok"}`,
		`{"version":null,"reason":"ok"}`, `{"version":1,"reason":null}`,
		actionValid + ` true`,
	}
	for i, raw := range actionBad {
		t.Run(fmt.Sprintf("action_%d", i), func(t *testing.T) {
			if json.Unmarshal([]byte(raw), &ActionInput{}) == nil {
				t.Fatalf("accepted body %s", raw)
			}
		})
	}
}

func TestInputValidationCanonicalIDsAndReasons(t *testing.T) {
	base := GrantInput{MemberID: testMember, Points: 1, Reason: "ok"}
	if !canonicalUUID(testMember) || !validID(testMember) {
		t.Fatal("lowercase canonical UUID rejected")
	}
	for _, id := range []string{
		strings.ToUpper(testMember), " " + testMember, "33333333333343338333111111111111", "not-a-uuid",
	} {
		in := base
		in.MemberID = id
		if in.Validate() == nil {
			t.Errorf("accepted invalid member ID %q", id)
		}
	}
	for _, reason := range []string{"", " ", " leading", "trailing ", "line\nbreak", "tab\there", "nul\x00", "unicode\u0085control", strings.Repeat("x", 501)} {
		in := base
		in.Reason = reason
		if in.Validate() == nil {
			t.Errorf("accepted invalid reason %q", reason)
		}
	}
	base.Reason = strings.Repeat("界", 166) // 498 UTF-8 bytes.
	if err := base.Validate(); err != nil {
		t.Fatalf("valid UTF-8 reason rejected: %v", err)
	}
	base.Reason += "界" // 501 UTF-8 bytes.
	if base.Validate() == nil {
		t.Fatal("accepted reason over 500 bytes")
	}
	if !errors.Is(ErrInvalid, ErrInvalid) {
		t.Fatal("ErrInvalid sentinel unavailable")
	}
}

func TestDTOJSONUsesExplicitSnakeCaseKeysAndNullPointers(t *testing.T) {
	at := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	order := Order{ID: "id", BrandID: "brand", MemberID: "member", Points: 9007199254740993,
		State: "granted", Version: MaxVersion, CreatedAt: at, UpdatedAt: at}
	raw, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	expected := []string{"id", "brand_id", "member_id", "points", "state", "version", "grant_ledger_entry_id", "revoke_ledger_entry_id", "creation_audit_log_id", "last_audit_log_id", "last_error_code", "created_by", "reason", "point_policy_version", "created_at", "updated_at", "revoked_at"}
	if len(fields) != len(expected) {
		t.Fatalf("unexpected order key count: got %d keys in %s", len(fields), raw)
	}
	for _, key := range expected {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing JSON key %q in %s", key, raw)
		}
	}
	if string(fields["points"]) != `"9007199254740993"` || string(fields["version"]) != fmt.Sprint(MaxVersion) {
		t.Fatalf("lossy numeric serialization: %s", raw)
	}
	if string(fields["revoke_ledger_entry_id"]) != "null" || string(fields["last_error_code"]) != "null" || string(fields["revoked_at"]) != "null" {
		t.Fatalf("nil pointers should be explicit nulls: %s", raw)
	}

	action, err := json.Marshal(Action{ID: "a", BrandID: "b", OrderID: "o", StateBefore: nil, LedgerEntryID: nil})
	if err != nil || !strings.Contains(string(action), `"state_before":null`) || !strings.Contains(string(action), `"ledger_entry_id":null`) {
		t.Fatalf("action DTO omitted nil fields: %s (%v)", action, err)
	}
}

func TestAllowedRewardPermissionsAreExactAndScoped(t *testing.T) {
	permission := func(action string, scope access.Scope) access.Permission {
		return access.Permission{Resource: "reward", Action: action, Scope: scope}
	}
	viewGrant := access.Role{BrandID: testBrand, Permissions: []access.Permission{permission("view", access.ScopeBrand)}}
	grantGrant := access.Role{BrandID: testBrand, Permissions: []access.Permission{permission("grant", access.ScopeBrand)}}
	actor := access.Account{Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{viewGrant, grantGrant}}
	for _, action := range []string{"view", "grant"} {
		if !Allowed(actor, testBrand, action) {
			t.Errorf("expected authorization for %q from separate roles", action)
		}
	}
	for _, action := range []string{"read", "revoke", "retry", "write", ""} {
		if Allowed(actor, testBrand, action) {
			t.Errorf("unexpected authorization for %q", action)
		}
	}
	if Allowed(actor, testBrand2, "view") || Allowed(actor, testBrand2, "grant") {
		t.Fatal("permissions crossed brand boundaries")
	}

	platformRead := access.Account{Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{permission("view", access.ScopePlatform)}}}}
	if !Allowed(platformRead, testBrand, "view") {
		t.Fatal("platform view did not permit view")
	}
	platformViewBrandMutation := access.Account{Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{
		{Permissions: []access.Permission{permission("view", access.ScopePlatform), permission("retry", access.ScopeBrand)}},
	}}
	if !Allowed(platformViewBrandMutation, testBrand, "retry") {
		t.Fatal("platform view did not combine with brand-scoped retry")
	}

	for _, resource := range []string{"wallet", "commission", "recharge"} {
		inherited := access.Account{Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{
			{Resource: resource, Action: "view", Scope: access.ScopeBrand},
			{Resource: resource, Action: "grant", Scope: access.ScopeBrand},
		}}}}
		if Allowed(inherited, testBrand, "view") || Allowed(inherited, testBrand, "grant") {
			t.Errorf("inherited access from %s", resource)
		}
	}

	for _, mutation := range []string{"grant", "revoke", "retry"} {
		admin := access.Account{Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{
			permission("view", access.ScopeBrand), permission(mutation, access.ScopeBrand),
		}}}}
		if !Allowed(admin, testBrand, mutation) {
			t.Errorf("valid %s grant denied", mutation)
		}
		admin.SuperAdmin = true
		if Allowed(admin, testBrand, mutation) {
			t.Errorf("super admin allowed to %s", mutation)
		}
		admin.SuperAdmin = false
		admin.Type = access.AccountUser
		if Allowed(admin, testBrand, mutation) {
			t.Errorf("non-admin allowed to %s", mutation)
		}
	}
	reader := platformRead
	reader.SuperAdmin = true
	if !Allowed(reader, testBrand, "view") {
		t.Fatal("super admin with explicit platform view should be able to read")
	}
	if Allowed(actor, strings.ToUpper(testBrand), "view") || Allowed(actor, "not-a-uuid", "view") {
		t.Fatal("non-canonical brand identifier allowed")
	}
}

func TestInputValidationErrorsWrapErrInvalid(t *testing.T) {
	if err := (ActionInput{Version: MaxVersion + 1, Reason: "ok"}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
	if err := (GrantInput{MemberID: testMember, Points: points.Amount(0), Reason: "ok"}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
