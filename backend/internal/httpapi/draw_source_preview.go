package httpapi

import (
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"net/http"
)

func registerDrawSourcePreview(register func(string, string, http.HandlerFunc), d Dependencies) {
	register("POST", "/platform-games/{id}/draw-source-preview", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Plan     platformlottery.AutomationPlan `json:"plan"`
			Sample   string                         `json:"sample"`
			PeriodNo string                         `json:"period_no"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		s := platformlottery.Service{DB: d.Admins.DB}
		detail, e := s.Detail(r.Context(), "", id, 1, 0)
		var out platformlottery.FeedReading
		if e == nil {
			if in.Plan.SourceType != "mapped" || platformlottery.ValidateAutomation(in.Plan) != nil {
				e = platformlottery.ErrInvalid
			} else if in.Sample != "" {
				out, e = platformlottery.ParseFeed(*in.Plan.Mapping, []byte(in.Sample))
			} else {
				sources, err := platformlottery.LoadAutomationSources()
				if err != nil {
					e = err
				} else if !sources.Allows(in.Plan.SourceURL) {
					failure(w, r, 400, "AUTOMATION_SOURCE_NOT_ALLOWED", "Source origin is not approved")
					return
				} else {
					out, e = platformlottery.FetchMappedFeed(r.Context(), sources, in.Plan, in.PeriodNo, detail.Game.Code)
				}
			}
		}
		if e == nil && out.Completed != nil {
			if rules.ValidateDraw(detail.Game.Model, out.Completed.Result) != nil {
				e = platformlottery.ErrInvalid
			}
		}
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.draw_source.preview") {
			return
		}
		var timeError *platformlottery.FeedTimeError
		if errors.As(e, &timeError) && d.Logger != nil {
			d.Logger.Warn("draw source time mapping failure", "field", timeError.Field, "value", timeError.Value)
		}
		var browserError *platformlottery.BrowserRenderError
		if errors.As(e, &browserError) && d.Logger != nil {
			d.Logger.Warn("draw source browser failure", "game_id", id, "diagnostic", browserError.Diagnostic)
		}
		result, e := platformLotteryResult(200, out, e)
		outputMutation(w, r, result, e)
	})
}
