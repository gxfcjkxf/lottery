package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
)

// Uploaded images are immutable and content-addressed within a brand. Retrying
// an upload returns the same asset without creating another business change.
func registerBrandLogoRoutes(mux routeRegistrar, d Dependencies) {
	mux.HandleFunc("GET /api/v1/brand-assets/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		id, ext, ok := strings.Cut(file, ".")
		if !ok || len(id) != 64 || (ext != "png" && ext != "jpg") {
			http.NotFound(w, r)
			return
		}
		if _, err := hex.DecodeString(id); err != nil {
			http.NotFound(w, r)
			return
		}
		if d.Admins.DB == nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "素材服务不可用")
			return
		}
		var media string
		var content []byte
		err := d.Admins.DB.QueryRow(r.Context(), `SELECT media_type,content FROM brand_logo_assets WHERE id=$1`, id).Scan(&media, &content)
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法读取素材")
			return
		}
		if ext == "png" && media != "image/png" || ext == "jpg" && media != "image/jpeg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", media)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(content)
	})
}

func registerBrandLogoUpload(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/brand-presentation/defaults", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := presentationActor(w, r, d, "view")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		record, err := (brandskin.Service{DB: d.Admins.DB}).Read(r.Context(), brand)
		if err != nil {
			result, e := presentationResult(nil, err)
			outputMutation(w, r, result, e)
			return
		}
		defaults, err := brandskin.Resolve(record.BaseName, brandskin.Config{})
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法读取默认展示设置")
			return
		}
		if adminReadAudit(w, r, d, a, brand, "brand_presentation.defaults") {
			respond(w, r, 200, defaults)
		}
	})
	handle("POST", "/brand-presentation/logo", func(w http.ResponseWriter, r *http.Request) {
		actor, brand, ok := presentationActor(w, r, d, "write")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		cfg, format, imageErr := image.DecodeConfig(bytes.NewReader(content))
		media, ext := "image/png", "png"
		if format == "jpeg" {
			media, ext = "image/jpeg", "jpg"
		}
		if err != nil || imageErr != nil || (format != "png" && format != "jpeg") || r.Header.Get("Content-Type") != media || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
			failure(w, r, 400, "BRAND_LOGO_INVALID", "图片须为不超过1MB、4096像素的PNG或JPG")
			return
		}
		// Decode the complete raster, not just its header, before publishing bytes.
		if _, _, err = image.Decode(bytes.NewReader(content)); err != nil {
			failure(w, r, 400, "BRAND_LOGO_INVALID", "图片无效")
			return
		}
		tx, err := d.Admins.DB.Begin(r.Context())
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法保存素材")
			return
		}
		defer tx.Rollback(r.Context())
		fresh, err := freshAdmin(r.Context(), tx, r, d, actor, false)
		if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化")
			return
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法核对素材权限")
			return
		}
		if !brandskin.Allowed(fresh, brand, "write") {
			rejectAdmin(w, r, d, fresh, brand, "brand_presentation.write")
			return
		}
		var status string
		if err = tx.QueryRow(r.Context(), `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
			failure(w, r, 404, "BRAND_PRESENTATION_NOT_FOUND", "品牌不存在")
			return
		} else if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法核对品牌状态")
			return
		}
		if status == "disabled" {
			failure(w, r, 409, "BRAND_PRESENTATION_STATE_CONFLICT", "停用品牌展示设置只读")
			return
		}
		digest := sha256.Sum256(append([]byte(brand+":"), content...))
		id := hex.EncodeToString(digest[:])
		url := "/api/v1/brand-assets/" + id + "." + ext
		inserted, err := tx.Exec(r.Context(), `INSERT INTO brand_logo_assets(id,brand_id,media_type,content) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, brand, media, content)
		if err == nil && inserted.RowsAffected() == 1 {
			_, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "brand_presentation.logo.upload", ResourceType: "brand", ResourceID: brand, RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"url": url}})
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法保存素材")
			return
		}
		respond(w, r, 200, map[string]string{"url": url})
	})
}
