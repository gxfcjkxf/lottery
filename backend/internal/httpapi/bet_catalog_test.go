package httpapi

import (
	"encoding/json"
	"testing"
)

func TestPublicCatalogIsTenantScopedAndDoesNotRequireLogin(t *testing.T) {
	f := managedFixture(t)
	for _, brandPath := range []string{"", "/b/harbor"} {
		read := f.call("GET", "/api/v1"+brandPath+"/platform-games", "", "", managedBrand, nil)
		mustStatus(t, read, 200)
		var out struct {
			Items []json.RawMessage `json:"games"`
			Limit int               `json:"limit"`
		}
		managedData(t, read, &out)
		if out.Items == nil || len(out.Items) != 0 || out.Limit != 50 {
			t.Fatalf("new brand catalog=%+v", out)
		}
	}
	bad := f.call("GET", "/api/v1/platform-games?limit=101", "", "", managedBrand, nil)
	mustStatus(t, bad, 400)
	badID := f.call("GET", "/api/v1/platform-games/not-a-uuid", "", "", managedBrand, nil)
	mustStatus(t, badID, 400)
	unknown := f.call("GET", "/api/v1/b/unknown/platform-games", "", "", managedBrand, nil)
	mustStatus(t, unknown, 404)
}
