package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"time"
)

type Dependencies struct {
	Brands         tenant.Resolver
	Ready          func(context.Context) error
	Logger         *slog.Logger
	Identity       *identity.Store
	Mutations      *mutation.Engine
	SecureCookies  bool
	Admins         adminsys.Store
	Telegram       telegramauth.Verifier
	TrustedProxies []*net.IPNet
}
type requestKey struct{}
type envelope struct {
	Success   bool      `json:"success"`
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string    `json:"request_id"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var requestPattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,80}$`)

func New(d Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, r, 200, map[string]string{"status": "alive"}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.Ready(ctx); err != nil {
			failure(w, r, 503, "SERVICE_NOT_READY", "数据库尚未就绪")
			return
		}
		respond(w, r, 200, map[string]string{"status": "ready"})
	})
	contextHandler := func(w http.ResponseWriter, r *http.Request) {
		brand, err := d.Brands.Resolve(r.Context(), r.Host, r.PathValue("brandCode"))
		if errors.Is(err, tenant.ErrNotFound) {
			failure(w, r, 404, "BRAND_NOT_FOUND", "当前入口没有可用品牌")
			return
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载品牌")
			return
		}
		cfg := identity.Settings{Privacy: "dev-1", Terms: "dev-1"}
		if d.Identity != nil {
			cfg, err = d.Identity.Settings(r.Context(), brand.ID)
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载配置")
				return
			}
		}
		respond(w, r, 200, map[string]any{"brand": brand, "available_locales": []string{"en", "zh-CN"}, "features": map[string]bool{"pwa": true, "real_payments": false}, "auth": map[string]any{"captcha_enabled": cfg.CaptchaEnabled, "telegram_enabled": cfg.TelegramEnabled, "telegram_client_id": cfg.TelegramClientID}, "terms": map[string]string{"privacy_policy_version": cfg.Privacy, "service_terms_version": cfg.Terms}})
	}
	mux.HandleFunc("GET /api/v1/context", contextHandler)
	mux.HandleFunc("GET /api/v1/b/{brandCode}/context", contextHandler)
	registerAuthRoutes(mux, d)
	registerAdminRoutes(mux, d)
	registerTelegramRoutes(mux, d)
	registerPointRoutes(mux, d)
	registerBetRoutes(mux, d)
	registerBetCatalogRoutes(mux, d)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { failure(w, r, 404, "NOT_FOUND", "接口不存在") })
	return middleware(d.Logger, d.TrustedProxies, mux)
}
func respond(w http.ResponseWriter, r *http.Request, status int, data any) {
	write(w, status, envelope{Success: true, Data: data, RequestID: requestID(r)})
}
func failure(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	write(w, status, envelope{Success: false, Error: &apiError{code, message}, RequestID: requestID(r)})
}
func write(w http.ResponseWriter, status int, data envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func requestID(r *http.Request) string { id, _ := r.Context().Value(requestKey{}).(string); return id }
func middleware(logger *slog.Logger, trusted []*net.IPNet, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !requestPattern.MatchString(id) {
			id = ids.New()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, id))
		r = r.WithContext(context.WithValue(r.Context(), clientIPKey{}, clientIP(r, trusted)))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panic", "request_id", id)
				failure(w, r, 500, "INTERNAL_ERROR", "服务发生异常")
			}
			logger.Info("http request", "request_id", id, "method", r.Method, "duration_ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(w, r)
	})
}
