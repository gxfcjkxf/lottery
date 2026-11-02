package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/branddomains"
	"testing"
)

func TestPlatformDomainHTTPWritesReplayAndIsolation(t *testing.T) {
	f := managedFixture(t)
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		path := "/api/v1/admin/brand-domains"
		if method == "PATCH" || method == "DELETE" {
			path += "/" + managedBrand
		}
		mustStatus(t, f.call(method, path, "brand-domain-retired-"+method, f.token, managedBrand, nil), 404)
	}
	op := securityOperator(t, f)
	const path = "/api/v1/platform/brand-domains"
	mustStatus(t, op.call("GET", path, "", op.token, managedBrand, nil), 403)
	role := grantPlatformPermission(t, op, "brand_domains.view.platform", "brand_domains.write.platform")
	var before branddomains.Record
	managedData(t, op.call("GET", path, "", op.token, managedBrand, nil), &before)
	in := branddomains.Input{Version: before.Version, Domain: "platform-domain.example.test", Enabled: true}
	first := op.call("POST", path, "platform-domain-add-01", op.token, managedBrand, in)
	mustStatus(t, first, 201)
	var added, replayed branddomains.Record
	managedData(t, first, &added)
	managedData(t, op.call("POST", path, "platform-domain-add-01", op.token, managedBrand, in), &replayed)
	if added.Version != replayed.Version || added.AuditLogID != replayed.AuditLogID {
		t.Fatal("replay created a second mutation")
	}
	var domainID string
	for _, d := range added.Domains {
		if d.Domain == in.Domain {
			domainID = d.ID
		}
	}
	if domainID == "" {
		t.Fatal("new domain absent")
	}
	const other = "0199a000-0000-7000-8000-000000000002"
	var foreign branddomains.Record
	managedData(t, op.call("GET", path, "", op.token, other, nil), &foreign)
	mustStatus(t, op.call("PATCH", path+"/"+domainID, "platform-domain-foreign", op.token, other, branddomains.Input{Version: foreign.Version, Enabled: false}), 404)
	mustStatus(t, f.call("POST", path, "platform-domain-staff", f.token, managedBrand, in), 403)
	var changed branddomains.Record
	managedData(t, op.call("PATCH", path+"/"+domainID, "platform-domain-edit-01", op.token, managedBrand, branddomains.Input{Version: added.Version, Enabled: false}), &changed)
	mustStatus(t, op.call("DELETE", path+"/"+domainID, "platform-domain-delete", op.token, managedBrand, map[string]int64{"version": changed.Version}), 200)
	if _, err := op.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='brand_domains.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, op.call("POST", path, "platform-domain-add-01", op.token, managedBrand, in), 403)
}
