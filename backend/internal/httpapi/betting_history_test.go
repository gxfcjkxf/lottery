package httpapi

import (
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"testing"
)

func TestBettingHistoryHTTPMembershipAndInput(t *testing.T) {
	f := pointsFixture(t)
	for _, path := range []string{"/api/v1/betting-history", "/api/v1/b/aurora/betting-history"} {
		r := f.call("GET", path, "", f.userToken, managedBrand, nil)
		mustStatus(t, r, 200)
		var page betting.HistoryPage
		managedData(t, r, &page)
		if page.BrandID != managedBrand || page.MemberID != f.memberID || len(page.Items) != 0 || len(page.Games) != 0 || page.NextCursor != nil {
			t.Fatal(page)
		}
		mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
		for _, query := range []string{"?member_id=" + f.memberID, "?offset=0", "?limit=0", "?limit=1&limit=2", "?game_id=bad", "?cursor=bad"} {
			mustStatus(t, f.call("GET", path+query, "", f.userToken, managedBrand, nil), 400)
		}
	}
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/betting-history", "", f.userToken, "", nil), 401)
}
