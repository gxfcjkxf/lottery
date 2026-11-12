package httpapi

import "testing"

func TestRetiredTronKeyEndpoints(t *testing.T) {
	f := managedFixture(t)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/v1/platform/cash/tron/keys"}, {"POST", "/api/v1/platform/cash/tron/keys"}} {
		mustStatus(t, f.call(endpoint.method, endpoint.path, "retired-key", f.token, "", nil), 404)
	}
}
