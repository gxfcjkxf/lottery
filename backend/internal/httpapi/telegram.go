package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func registerTelegramRoutes(mux *http.ServeMux, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/auth/telegram/challenge", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			if d.Identity == nil || d.Mutations == nil {
				failure(w, r, 503, "AUTH_UNAVAILABLE", "认证未配置")
				return
			}
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			cfg, err := d.Identity.Settings(r.Context(), b.ID)
			if err != nil || !cfg.TelegramEnabled || cfg.TelegramClientID == "" {
				failure(w, r, 403, "TELEGRAM_DISABLED", "Telegram 登录未配置")
				return
			}
			allow, err := d.Identity.AllowAttempt(r.Context(), d.Mutations, b.ID, "", meta(r))
			if err != nil || !allow {
				failure(w, r, 429, "AUTH_RATE_LIMITED", "请求过于频繁")
				return
			}
			challenge, err := d.Identity.Challenge(r.Context(), d.Mutations, b.ID, "telegram")
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "登录挑战暂不可用")
				return
			}
			respond(w, r, 200, challenge)
		})
		mux.HandleFunc("POST "+prefix+"/auth/telegram", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			if d.Identity == nil || d.Mutations == nil {
				failure(w, r, 503, "AUTH_UNAVAILABLE", "认证未配置")
				return
			}
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in identity.TelegramInput
			if !decodeBody(w, r, &in) {
				return
			}
			var bindUser string
			if in.Bind {
				v, err := d.Identity.Authenticate(r.Context(), b.ID, requestToken(r, userCookieName(b.ID)))
				if err != nil {
					failure(w, r, 401, "AUTH_SESSION_REVOKED", "绑定需要先登录")
					return
				}
				bindUser = v.User.ID
			}
			encoded, _ := json.Marshal(in)
			result, err := d.Mutations.Execute(r.Context(), b.ID, "telegram:"+bindUser, "auth.telegram", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				allow, err := d.Identity.AllowAttempt(ctx, d.Mutations, b.ID, "", meta(r))
				if err != nil {
					return mutation.Result{}, err
				}
				if !allow {
					return mutation.Fail(429, "AUTH_RATE_LIMITED", "请求过于频繁"), nil
				}
				cfg, err := d.Identity.Settings(ctx, b.ID)
				if err != nil {
					return mutation.Result{}, err
				}
				if !cfg.TelegramEnabled || cfg.TelegramClientID == "" {
					return mutation.Fail(403, "TELEGRAM_DISABLED", "Telegram 登录未配置"), nil
				}
				valid, err := d.Identity.Consume(ctx, d.Mutations, b.ID, "telegram", in.ChallengeID, in.Nonce)
				if err != nil {
					return mutation.Result{}, err
				}
				if !valid {
					return mutation.Fail(401, "TELEGRAM_CHALLENGE_INVALID", "登录挑战已失效"), nil
				}
				claims, err := d.Telegram.Verify(ctx, in.IDToken, cfg.TelegramClientID, in.Nonce, time.Now())
				if err == telegramauth.ErrKeyUnavailable {
					return mutation.Fail(503, "TELEGRAM_UNAVAILABLE", "Telegram 验证服务暂不可用，请重新授权"), nil
				}
				if err != nil {
					return mutation.Fail(401, "TELEGRAM_INVALID_TOKEN", "Telegram 授权无效"), nil
				}
				allow, err = d.Identity.AllowAccountAttempt(ctx, d.Mutations, b.ID, "telegram:"+claims.ID, meta(r))
				if err != nil {
					return mutation.Result{}, err
				}
				if !allow {
					return mutation.Fail(429, "AUTH_RATE_LIMITED", "尝试次数过多"), nil
				}
				return d.Identity.Telegram(ctx, tx, b.ID, claims, in, bindUser, meta(r))
			})
			if err == nil && result.Error == nil {
				var auth identity.Authentication
				if json.Unmarshal(result.Data, &auth) == nil {
					issueCookie(w, r, userCookieName(b.ID), auth.AccessToken, auth.ExpiresAt, d.SecureCookies)
				}
			}
			outputMutation(w, r, result, err)
		})
	}
}
