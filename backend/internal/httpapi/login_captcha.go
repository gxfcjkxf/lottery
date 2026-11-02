package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"net/http"
)

func checkLoginCaptcha(ctx context.Context, d Dependencies, brand, id, answer string) (*mutation.Result, error) {
	cfg, err := d.Identity.Settings(ctx, brand)
	if err != nil {
		return nil, err
	}
	if !cfg.CaptchaEnabled {
		return nil, nil
	}
	valid, err := d.Identity.Consume(ctx, d.Mutations, brand, "captcha", id, answer)
	if err != nil {
		return nil, err
	}
	if !valid {
		result := mutation.Fail(400, "CAPTCHA_INVALID", "CAPTCHA_INVALID")
		return &result, nil
	}
	return nil, nil
}
func loginChallenge(w http.ResponseWriter, r *http.Request, d Dependencies, brand string) {
	allowed, err := d.Identity.AllowAttempt(r.Context(), d.Mutations, brand, "", meta(r))
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "SERVICE_UNAVAILABLE")
		return
	}
	if !allowed {
		failure(w, r, 429, "AUTH_RATE_LIMITED", "AUTH_RATE_LIMITED")
		return
	}
	challenge, err := d.Identity.Challenge(r.Context(), d.Mutations, brand, "captcha")
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "SERVICE_UNAVAILABLE")
		return
	}
	respond(w, r, 200, challenge)
}
