package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/announcement"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
	"image"
	"io"
	"net/http"
	"strings"
	"time"
)

func announcementResult(v any, e error) (mutation.Result, error) {
	switch {
	case errors.Is(e, announcement.ErrInvalid):
		return mutation.Fail(400, "ANNOUNCEMENT_INVALID", "公告内容或设置无效"), nil
	case errors.Is(e, announcement.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无公告操作权限"), nil
	case errors.Is(e, announcement.ErrConflict):
		return mutation.Fail(409, "ANNOUNCEMENT_CONFLICT", "公告版本或品牌状态已变化，请刷新"), nil
	case errors.Is(e, announcement.ErrNotFound):
		return mutation.Fail(404, "ANNOUNCEMENT_NOT_FOUND", "公告不存在"), nil
	case errors.Is(e, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "会话已失效，请重新登录"), nil
	case e != nil:
		return mutation.Result{}, e
	default:
		return mutation.OK(200, v), nil
	}
}
func announcementActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	brand := ""
	if !platformAdminEntry(r) {
		brand = r.Header.Get("X-Brand-ID")
		if !announcement.ValidID(brand) {
			failure(w, r, 400, "ANNOUNCEMENT_INVALID", "请选择品牌")
			return a, brand, false
		}
	}
	if !announcement.Allowed(a, brand, action) {
		rejectAdmin(w, r, d, a, brand, "announcement."+action)
		return a, brand, false
	}
	return a, brand, true
}
func announcementFresh(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, a access.Account, brand, action string) (access.Account, error) {
	fresh, e := freshAdmin(ctx, tx, r, d, a, brand == "")
	if errors.Is(e, adminsys.ErrDenied) {
		e = identity.ErrSession
	}
	if e != nil {
		return fresh, e
	}
	if !announcement.Allowed(fresh, brand, action) {
		return fresh, announcement.ErrDenied
	}
	return fresh, nil
}
func registerAnnouncementAdmin(handle func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	handle("GET", "/announcements", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := announcementActor(w, r, d, "view")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "ANNOUNCEMENT_INVALID", "不支持的查询参数")
			return
		}
		tx, e := d.Admins.DB.Begin(r.Context())
		if e != nil {
			outputMutation(w, r, mutation.Result{}, e)
			return
		}
		defer tx.Rollback(r.Context())
		_, e = announcementFresh(r.Context(), tx, r, d, a, brand, "view")
		var items []announcement.Item
		if e == nil {
			items, e = announcement.List(r.Context(), tx, brand)
		}
		if e == nil {
			_, e = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "announcement.list", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
		}
		if e == nil {
			e = tx.Commit(r.Context())
		}
		out, e := announcementResult(map[string]any{"items": items}, e)
		outputMutation(w, r, out, e)
	})
	if platform {
		handle("GET", "/announcements/brands", func(w http.ResponseWriter, r *http.Request) {
			_, brand, ok := announcementActor(w, r, d, "view")
			if !ok {
				return
			}
			if brand != "" {
				failure(w, r, 403, "PERMISSION_DENIED", "仅平台可选择品牌")
				return
			}
			rows, e := d.Admins.DB.Query(r.Context(), `SELECT id::text,name FROM brands ORDER BY name,id`)
			if e != nil {
				outputMutation(w, r, mutation.Result{}, e)
				return
			}
			defer rows.Close()
			items := []map[string]string{}
			for rows.Next() {
				var id, name string
				if e = rows.Scan(&id, &name); e != nil {
					break
				}
				items = append(items, map[string]string{"id": id, "name": name})
			}
			if e == nil {
				e = rows.Err()
			}
			out, e := announcementResult(map[string]any{"items": items}, e)
			outputMutation(w, r, out, e)
		})
	}

	for _, route := range []struct{ method, path string }{{"POST", "/announcements"}, {"PUT", "/announcements/{id}"}} {
		handle(route.method, route.path, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := announcementActor(w, r, d, "write")
			if !ok {
				return
			}
			id := r.PathValue("id")
			if id != "" && !announcement.ValidID(id) {
				failure(w, r, 400, "ANNOUNCEMENT_INVALID", "公告编号无效")
				return
			}
			var in announcement.Input
			if !decodeBodyLimit(w, r, &in, 256<<10) {
				return
			}
			if _, e := announcement.Validate(&in, brand); e != nil {
				out, e := announcementResult(nil, e)
				outputMutation(w, r, out, e)
				return
			}
			raw, _ := json.Marshal(in)
			var fresh access.Account
			out, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "announcement.save", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				var e error
				fresh, e = announcementFresh(ctx, tx, r, d, a, brand, "write")
				if e != nil {
					return e
				}
				if in.Status == "published" || in.Status == "withdrawn" {
					if !announcement.Allowed(fresh, brand, "publish") {
						return announcement.ErrDenied
					}
				}
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				v, e := announcement.Save(ctx, tx, fresh, brand, id, in, pointMeta(r, fresh))
				return announcementResult(v, e)
			})
			if errors.Is(e, announcement.ErrDenied) || errors.Is(e, identity.ErrSession) {
				out, e = announcementResult(nil, e)
			}
			finishAdminMutation(w, r, d, a, brand, "announcement.save", out, e)
		})
	}
	handle("POST", "/announcements/image", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := announcementActor(w, r, d, "write")
		if !ok {
			return
		}
		content, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
		cfg, format, ie := image.DecodeConfig(bytes.NewReader(content))
		media, ext := "", ""
		switch format {
		case "png":
			media, ext = "image/png", "png"
		case "jpeg":
			media, ext = "image/jpeg", "jpg"
		case "webp":
			media, ext = "image/webp", "webp"
		}
		if e != nil || ie != nil || media == "" || r.Header.Get("Content-Type") != media || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
			failure(w, r, 400, "ANNOUNCEMENT_IMAGE_INVALID", "图片须为不超过5MB、4096像素的PNG、JPG或WebP")
			return
		}
		if _, _, e = image.Decode(bytes.NewReader(content)); e != nil {
			failure(w, r, 400, "ANNOUNCEMENT_IMAGE_INVALID", "图片无效")
			return
		}
		tx, e := d.Admins.DB.Begin(r.Context())
		if e != nil {
			outputMutation(w, r, mutation.Result{}, e)
			return
		}
		defer tx.Rollback(r.Context())
		_, e = announcementFresh(r.Context(), tx, r, d, a, brand, "write")
		if e == nil && brand != "" {
			var status string
			e = tx.QueryRow(r.Context(), `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status)
			if status == "disabled" {
				e = announcement.ErrConflict
			}
		}
		digest := sha256.Sum256(append([]byte(brand+":"), content...))
		id := hex.EncodeToString(digest[:])
		url := "/api/v1/announcement-assets/" + id + "." + ext
		if e == nil {
			_, e = tx.Exec(r.Context(), `INSERT INTO announcement_assets(id,brand_id,media_type,content) VALUES($1,NULLIF($2,'')::uuid,$3,$4) ON CONFLICT DO NOTHING`, id, brand, media, content)
		}
		if e == nil {
			_, e = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "announcement.image.upload", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"url": url}})
		}
		if e == nil {
			e = tx.Commit(r.Context())
		}
		out, e := announcementResult(map[string]string{"url": url}, e)
		outputMutation(w, r, out, e)
	})
}
func announcementSubject(w http.ResponseWriter, r *http.Request, d Dependencies, brand string) (announcement.Subject, bool) {
	name := "lottery_announcements_" + brand
	visitor := ""
	if c, e := r.Cookie(name); e == nil {
		visitor = c.Value
	}
	if !announcement.ValidID(visitor) {
		visitor = ids.New()
		http.SetCookie(w, &http.Cookie{Name: name, Value: visitor, Path: "/api/v1", HttpOnly: true, Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 31536000})
	}
	session := r.Header.Get("X-Announcement-Session")
	if !announcement.ValidID(session) {
		failure(w, r, 400, "ANNOUNCEMENT_INVALID", "公告会话无效")
		return announcement.Subject{}, false
	}
	s := announcement.Subject{Visitor: visitor, Session: session, BrowserSession: session}
	token := requestToken(r, userCookieName(brand))
	if token != "" {
		profile, e := d.Identity.Authenticate(r.Context(), brand, token)
		if e != nil {
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "会员会话已失效")
			return s, false
		}
		s.Member = profile.Member.ID
		hash := sha256.Sum256([]byte(token))
		s.Session = hex.EncodeToString(hash[:])
	}
	return s, true
}
func registerPublicAnnouncements(mux routeRegistrar, d Dependencies) {
	mux.HandleFunc("GET /api/v1/announcement-assets/{file}", func(w http.ResponseWriter, r *http.Request) {
		path := "/api/v1/announcement-assets/" + r.PathValue("file")
		id := announcement.AssetID(path)
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if d.Admins.DB == nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "素材不可用")
			return
		}
		var media string
		var content []byte
		e := d.Admins.DB.QueryRow(r.Context(), `SELECT media_type,content FROM announcement_assets WHERE id=$1`, id).Scan(&media, &content)
		if errors.Is(e, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if e != nil {
			outputMutation(w, r, mutation.Result{}, e)
			return
		}
		extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[media]
		if !strings.HasSuffix(path, "."+extension) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", media)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(content)
	})
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		for _, method := range []string{"GET", "POST"} {
			path := "/announcements"
			if method == "POST" {
				path += "/receipts"
			}
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				brand, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				if d.Admins.DB == nil || d.Identity == nil {
					failure(w, r, 503, "SERVICE_UNAVAILABLE", "公告服务不可用")
					return
				}
				w.Header().Set("Cache-Control", "no-store")
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				subject, ok := announcementSubject(w, r, d, brand.ID)
				if !ok {
					return
				}
				tx, e := d.Admins.DB.Begin(ctx)
				if e != nil {
					outputMutation(w, r, mutation.Result{}, e)
					return
				}
				defer tx.Rollback(ctx)
				var result any
				if r.Method == "GET" {
					var items []announcement.Item
					items, e = announcement.Feed(ctx, tx, brand.ID, subject)
					for i := range items {
						items[i].Config.Targets = []string{}
					}
					result = map[string]any{"items": items}
				} else {
					var in announcement.ReceiptInput
					if !decodeBody(w, r, &in) {
						return
					}
					e = announcement.Record(ctx, tx, brand.ID, subject, in)
					result = map[string]bool{"recorded": true}
				}
				if e == nil {
					e = tx.Commit(ctx)
				}
				out, e := announcementResult(result, e)
				outputMutation(w, r, out, e)
			})
		}
	}
}
