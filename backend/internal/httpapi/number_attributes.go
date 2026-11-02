package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerNumberAttributes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	if platform {
		svc := platformlottery.Service{DB: d.Admins.DB}
		register("POST", "/platform-games/{id}/mark-six/attributes", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "write")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in platformlottery.MarkSixAttributesInput
			if !decodeBodyLimit(w, r, &in, 256<<10) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "write", "mark_six.attributes", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := svc.WriteMarkSixAttributes(ctx, tx, fresh, id, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
		return
	}
}
