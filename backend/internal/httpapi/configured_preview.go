package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

// This endpoint owns no database resources and never creates a sale, period or
// wallet mutation. All calculation inputs must be supplied in the draft.
type configuredPreviewInput struct {
	Snapshot json.RawMessage           `json:"snapshot"`
	Requests []rules.ConfiguredRequest `json:"requests"`
	Draw     *rules.Draw               `json:"draw,omitempty"`
}

func configuredPreview(ctx context.Context, in configuredPreviewInput) (any, error) {
	if in.Requests == nil {
		return nil, rules.ErrInvalid
	}
	e, err := rules.RestoreConfiguredEngine(in.Snapshot)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"definition_hash": e.DefinitionHash(), "snapshot_hash": e.SnapshotHash(), "price_keys": e.PriceKeys()}
	if len(in.Requests) == 0 && in.Draw == nil {
		return out, nil
	}
	q, err := e.Prepare(ctx, in.Requests)
	if err != nil {
		return nil, err
	}
	out["quote"] = q
	if in.Draw != nil {
		result, err := e.SettleWithContext(ctx, q, *in.Draw, e.DefaultSettlementContext())
		if err != nil {
			return nil, err
		}
		out["result"] = result
	}
	return out, nil
}

func registerConfiguredPreviewRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/platform-games/rule-preview/template", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := platformLotteryActor(w, r, d, "view"); !ok {
			return
		}
		if len(r.URL.Query()) != 1 || len(r.URL.Query()["template"]) != 1 {
			outputMutation(w, r, mutation.Fail(400, "RULE_INVALID", "Choose an explicit rule template"), nil)
			return
		}
		c, err := rules.ConfiguredTemplateCatalog(r.URL.Query().Get("template"))
		if err != nil {
			outputMutation(w, r, mutation.Fail(400, "RULE_INVALID", "Unknown rule template"), nil)
			return
		}
		out, err := platformLotteryResult(200, c, nil)
		outputMutation(w, r, out, err)
	})
	handle("POST", "/platform-games/rule-preview", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := platformLotteryActor(w, r, d, "view"); !ok {
			return
		}
		var in configuredPreviewInput
		if !decodeBodyLimit(w, r, &in, 4<<20) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := configuredPreview(ctx, in)
		if err != nil {
			outputMutation(w, r, mutation.Fail(400, "RULE_INVALID", "Invalid rule configuration or simulation input"), nil)
			return
		}
		outputMutation(w, r, mutation.OK(200, out), nil)
	})
}
