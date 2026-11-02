package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/hkjccalendar"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func registerOfficialCalendarSync(register func(string, string, http.HandlerFunc), d Dependencies) {
	source := d.OfficialCalendarSource
	if source == nil {
		source = hkjccalendar.NewSource()
	}
	register("POST", "/platform-games/official-calendar/sync", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "Query parameters are not supported")
			return
		}
		var in struct{}
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.calendar.sync", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := (hkjccalendar.Service{Source: source}).SyncNow(ctx, tx)
			if errors.Is(err, hkjccalendar.ErrBusy) {
				return mutation.Fail(409, "HKJC_CALENDAR_BUSY", "Calendar sync is already running"), nil
			}
			if err != nil && !errors.Is(err, hkjccalendar.ErrFetch) {
				return mutation.Result{}, err
			}
			meta := pointMeta(r, fresh)
			if _, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: fresh.ID, Action: "platform_game.calendar.sync", ResourceType: "hkjc_calendar", RequestID: meta.RequestID, IP: meta.IP, After: out}); err != nil {
				return mutation.Result{}, err
			}
			// A completed source failure is an explicit outcome, committed and replayed
			// with the attempt metadata, rather than an ambiguous transport failure.
			return mutation.OK(200, out), nil
		})
	})
}
