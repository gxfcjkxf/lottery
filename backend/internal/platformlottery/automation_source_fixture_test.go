package platformlottery

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// Current mapped-source fixtures publish through the same named-source lifecycle
// as the management UI; the removed direct publication API is not recreated.
func publishTestAutomation(ctx context.Context, s Service, tx pgx.Tx, actor access.Account, game string, in AutomationInput, meta points.Metadata) (AutomationPlan, error) {
	source, err := s.SaveDrawSource(ctx, tx, actor, game, DrawSourceInput{DrawSource: DrawSource{Name: "test-source-" + ids.New(), Plan: in.AutomationPlan}}, meta)
	if err != nil {
		return AutomationPlan{}, err
	}
	return s.ActivateDrawSource(ctx, tx, actor, game, source.ID, DrawSourceActivation{Revision: source.Revision, Version: in.Version}, meta)
}
