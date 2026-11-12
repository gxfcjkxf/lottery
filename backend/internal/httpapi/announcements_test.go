package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	announcement "github.com/gxfcjkxf/lottery/backend/internal/announcement"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func grantAnnouncements(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, key := range []string{"announcement.view.brand", "announcement.write.brand", "announcement.publish.brand"} {
		if _, e := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
}
func announcementDraft(title string) announcement.Input {
	return announcement.Input{Status: "published", Config: announcement.Config{Content: announcement.Content{En: announcement.Copy{Title: title, HTML: "<p>English <strong>notice</strong></p>"}, Zh: announcement.Copy{Title: title, HTML: "<p>中文公告</p>"}}, Targets: []string{}, Audience: "all", Popup: true, Frequency: "daily", IntervalDays: 1, AllowMute: true}}
}
func saveAnnouncementHTTP(t *testing.T, f managementHTTP, base string, in announcement.Input) announcement.Item {
	t.Helper()
	w := f.call("POST", base, "announcement-"+ids.New(), f.token, managedBrand, in)
	mustStatus(t, w, 200)
	var out announcement.Item
	managedData(t, w, &out)
	return out
}
func TestAnnouncementsHTTPPermissionsReplayAndScope(t *testing.T) {
	f := managedFixture(t)
	base := "/api/v1/admin/announcements"
	in := announcementDraft("Brand notice")
	mustStatus(t, f.call("POST", base, "announcement-denied", f.token, managedBrand, in), 403)
	grantAnnouncements(t, f)
	first := f.call("POST", base, "announcement-create", f.token, managedBrand, in)
	mustStatus(t, first, 200)
	var v announcement.Item
	managedData(t, first, &v)
	if v.BrandID == nil || *v.BrandID != managedBrand || v.State != "active" {
		t.Fatal(v)
	}
	replay := f.call("POST", base, "announcement-create", f.token, managedBrand, in)
	mustStatus(t, replay, 200)
	var same announcement.Item
	managedData(t, replay, &same)
	if same.ID != v.ID {
		t.Fatal("duplicate announcement")
	}
	in.Version = 0
	mustStatus(t, f.call("PUT", base+"/"+v.ID, "announcement-stale", f.token, managedBrand, in), 409)
	mustStatus(t, f.call("GET", base, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	in.Version = v.Version
	in.Config.Content.En.HTML = `<script>alert(1)</script>`
	mustStatus(t, f.call("PUT", base+"/"+v.ID, "announcement-script", f.token, managedBrand, in), 400)
	_, e := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='announcement.publish.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root)
	if e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", base, "announcement-create", f.token, managedBrand, announcementDraft("Brand notice")), 403)
}
func TestAnnouncementsPlatformPriorityTargetingMuteAndNewPublication(t *testing.T) {
	f := managedFixture(t)
	grantAnnouncements(t, f)
	platform := securityOperator(t, f)
	grantPlatformPermission(t, platform, "announcement.view.platform", "announcement.write.platform", "announcement.publish.platform")
	brand := announcementDraft("Brand pinned")
	brand.Config.Pinned = true
	b := saveAnnouncementHTTP(t, f, "/api/v1/admin/announcements", brand)
	p := saveAnnouncementHTTP(t, platform, "/api/v1/platform/announcements", announcementDraft("Platform ordinary"))
	targeted := announcementDraft("Other brand")
	targeted.Config.Targets = []string{"0199a000-0000-7000-8000-000000000002"}
	saveAnnouncementHTTP(t, platform, "/api/v1/platform/announcements", targeted)
	guest := announcement.Subject{Visitor: ids.New(), Session: ids.New()}
	ctx := context.Background()
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	feed, e := announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil || len(feed) != 2 || feed[0].ID != p.ID || feed[1].ID != b.ID {
		t.Fatal("priority or targeting", feed, e)
	}
	if e = announcement.Record(ctx, tx, managedBrand, guest, announcement.ReceiptInput{Action: "view", Items: []announcement.Receipt{{ID: p.ID, Generation: p.Generation}}}); e != nil {
		t.Fatal(e)
	}
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil || feed[0].Eligible || !feed[1].Eligible {
		t.Fatal("view must not consume unseen page", feed, e)
	}
	receipts := []announcement.Receipt{{ID: p.ID, Generation: p.Generation}, {ID: b.ID, Generation: b.Generation}}
	if e = announcement.Record(ctx, tx, managedBrand, guest, announcement.ReceiptInput{Action: "mute", Items: receipts}); e != nil {
		t.Fatal(e)
	}
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil || feed[0].Eligible || feed[1].Eligible {
		t.Fatal("group mute", feed, e)
	}
	guest.Member = ids.New()
	if e = announcement.Sync(ctx, tx, managedBrand, guest); e != nil {
		t.Fatal(e)
	}
	other := guest
	other.Visitor = ids.New()
	feed, e = announcement.Feed(ctx, tx, managedBrand, other)
	if e != nil || feed[0].Eligible || feed[1].Eligible {
		t.Fatal("member sync", feed, e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	fresh := saveAnnouncementHTTP(t, platform, "/api/v1/platform/announcements", announcementDraft("New later publication"))
	tx, e = f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range feed {
		if v.ID == fresh.ID {
			found = v.Eligible
		}
	}
	if !found {
		t.Fatal("new publication suppressed by old group mute")
	}
}
func TestAnnouncementsFrequenciesScheduleAndForeignAsset(t *testing.T) {
	f := managedFixture(t)
	grantAnnouncements(t, f)
	ctx := context.Background()
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	a := access.Account{ID: f.root, Type: access.AccountAdmin, BrandIDs: []string{managedBrand}, Roles: []access.Role{{BrandID: managedBrand, Permissions: []access.Permission{{Resource: "announcement", Action: "write", Scope: access.ScopeBrand}, {Resource: "announcement", Action: "publish", Scope: access.ScopeBrand}}}}}
	guest := announcement.Subject{Visitor: ids.New(), Session: ids.New()}
	for _, frequency := range []string{"session", "daily", "interval", "once"} {
		in := announcementDraft(frequency)
		in.Config.Frequency = frequency
		in.Config.IntervalDays = 3
		v, e := announcement.Save(ctx, tx, a, managedBrand, "", in, points.Metadata{RequestID: ids.New()})
		if e != nil {
			t.Fatal(e)
		}
		if e = announcement.Record(ctx, tx, managedBrand, guest, announcement.ReceiptInput{Action: "view", Items: []announcement.Receipt{{ID: v.ID, Generation: v.Generation}}}); e != nil {
			t.Fatal(e)
		}
	}
	feed, e := announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range feed {
		if v.Eligible {
			t.Fatal("frequency not applied", v)
		}
	}
	guest.Session = ids.New()
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range feed {
		if v.Eligible != (v.Config.Frequency == "session") {
			t.Fatal("session boundary", v)
		}
	}
	_, e = tx.Exec(ctx, `UPDATE announcement_receipts SET seen_at=now()-interval '4 days' WHERE brand_id=$1`, managedBrand)
	if e != nil {
		t.Fatal(e)
	}
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range feed {
		if v.Eligible == (v.Config.Frequency == "once") {
			t.Fatal("interval boundary", v)
		}
	}
	in := announcementDraft("Future")
	future := time.Now().Add(time.Hour)
	in.Config.StartsAt = &future
	_, e = announcement.Save(ctx, tx, a, managedBrand, "", in, points.Metadata{RequestID: ids.New()})
	if e != nil {
		t.Fatal(e)
	}
	feed, e = announcement.Feed(ctx, tx, managedBrand, guest)
	if e != nil || len(feed) != 4 {
		t.Fatal("schedule", feed, e)
	}
	in = announcementDraft("Foreign asset")
	in.Config.Content.En.HTML = `<img src="/api/v1/announcement-assets/` + strings.Repeat("f", 64) + `.png">`
	if _, e = announcement.Save(ctx, tx, a, managedBrand, "", in, points.Metadata{}); e != announcement.ErrInvalid {
		t.Fatal("foreign asset", e)
	}
}

func TestAnnouncementsPublicCookiesImagesAndMemberLoginMerge(t *testing.T) {
	f := managedFixture(t)
	grantAnnouncements(t, f)
	ctx := context.Background()
	base := "/api/v1/admin/announcements"
	var raster bytes.Buffer
	if e := png.Encode(&raster, image.NewRGBA(image.Rect(0, 0, 8, 8))); e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", "http://localhost"+base+"/image", bytes.NewReader(raster.Bytes()))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("X-Brand-ID", managedBrand)
	req.Header.Set("Content-Type", "image/png")
	uploaded := httptest.NewRecorder()
	f.http.ServeHTTP(uploaded, req)
	mustStatus(t, uploaded, 200)
	var asset struct {
		URL string `json:"url"`
	}
	managedData(t, uploaded, &asset)
	imageResponse := f.call("GET", asset.URL, "", "", managedBrand, nil)
	mustStatus(t, imageResponse, 200)
	if imageResponse.Header().Get("Content-Type") != "image/png" || !bytes.Equal(imageResponse.Body.Bytes(), raster.Bytes()) {
		t.Fatal("image bytes or MIME changed")
	}
	in := announcementDraft("Session notice")
	in.Config.Frequency = "session"
	in.Config.Content.En.HTML = `<p><img src="` + asset.URL + `" alt="notice"></p>`
	notice := saveAnnouncementHTTP(t, f, base, in)
	session := ids.New()
	var cookie *http.Cookie
	public := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var content []byte
		if body != nil {
			content, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(content))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-Announcement-Session", session)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			cookie = c
		}
		return w
	}
	feed := func(token string) []announcement.Item {
		w := public("GET", "/api/v1/announcements", token, nil)
		mustStatus(t, w, 200)
		var data struct {
			Items []announcement.Item `json:"items"`
		}
		managedData(t, w, &data)
		return data.Items
	}
	if v := feed(""); len(v) != 1 || !v[0].Eligible {
		t.Fatal(v)
	}
	if cookie == nil || !cookie.HttpOnly {
		t.Fatal("visitor identity cookie missing")
	}
	receipt := announcement.ReceiptInput{Action: "view", Items: []announcement.Receipt{{ID: notice.ID, Generation: notice.Generation}}}
	mustStatus(t, public("POST", "/api/v1/announcements/receipts", "", receipt), 200)
	if feed("")[0].Eligible {
		t.Fatal("guest session frequency ignored")
	}
	registered := f.call("POST", "/api/v1/auth/register", "announcement-member-register", "", managedBrand, map[string]string{"username": "announcement_member", "password": "announcement-password-2026"})
	mustStatus(t, registered, 201)
	var auth identity.Authentication
	managedData(t, registered, &auth)
	if feed(auth.AccessToken)[0].Eligible {
		t.Fatal("login repeated visitor session announcement")
	}
	mustStatus(t, public("POST", "/api/v1/announcements/receipts", auth.AccessToken, announcement.ReceiptInput{Action: "sync", Items: []announcement.Receipt{}}), 200)
	cookie = nil
	if feed(auth.AccessToken)[0].Eligible {
		t.Fatal("member session missing after merging on another browser")
	}
	mustStatus(t, public("GET", "/api/v1/announcements", "invalid-token", nil), 401)
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM announcement_receipts WHERE subject=$1`, "m:"+auth.Member.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("member record not merged", count, e)
	}
}

func TestAnnouncementsUseCurrentPlatformTimezone(t *testing.T) {
	f := managedFixture(t)
	grantAnnouncements(t, f)
	v := saveAnnouncementHTTP(t, f, "/api/v1/admin/announcements", announcementDraft("Platform calendar"))
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE platform_settings SET timezone='Pacific/Kiritimati' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	// Old persisted JSON cannot override the platform's current calendar.
	if _, err = tx.Exec(ctx, `UPDATE announcements SET config=config || '{"timezone":"UTC"}'::jsonb WHERE id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	subject := announcement.Subject{Visitor: ids.New(), Session: ids.New()}
	receipt := announcement.ReceiptInput{Action: "view", Items: []announcement.Receipt{{ID: v.ID, Generation: v.Generation}}}
	if err = announcement.Record(ctx, tx, managedBrand, subject, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE announcement_receipts SET seen_at=(date_trunc('day',clock_timestamp() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 minute' WHERE announcement_id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	feed, err := announcement.Feed(ctx, tx, managedBrand, subject)
	if err != nil || len(feed) != 1 || !feed[0].Eligible {
		t.Fatal("previous platform day must be eligible", feed, err)
	}
	receipt.Action = "mute"
	if err = announcement.Record(ctx, tx, managedBrand, subject, receipt); err != nil {
		t.Fatal(err)
	}
	var correctDay bool
	if err = tx.QueryRow(ctx, `SELECT muted_day=to_char(clock_timestamp() AT TIME ZONE 'Pacific/Kiritimati','YYYY-MM-DD') FROM announcement_receipts WHERE announcement_id=$1`, v.ID).Scan(&correctDay); err != nil || !correctDay {
		t.Fatal("mute must use platform date", correctDay, err)
	}
	feed, err = announcement.Feed(ctx, tx, managedBrand, subject)
	if err != nil || feed[0].Eligible {
		t.Fatal("muted platform day", feed, err)
	}
	// A platform setting change is used by subsequent reads without republishing.
	if _, err = tx.Exec(ctx, `UPDATE platform_settings SET timezone='Etc/GMT+12' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	receipt.Action = "mute"
	if err = announcement.Record(ctx, tx, managedBrand, subject, receipt); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT muted_day=to_char(clock_timestamp() AT TIME ZONE 'Etc/GMT+12','YYYY-MM-DD') FROM announcement_receipts WHERE announcement_id=$1`, v.ID).Scan(&correctDay); err != nil || !correctDay {
		t.Fatal("updated platform date", correctDay, err)
	}
}
