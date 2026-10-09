package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"

	"github.com/gxfcjkxf/lottery/backend/internal/commissionreview"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
)

const commissionReviewConfirmation = "--confirm=offline-commission-review"

var canonicalLowerUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type commissionReviewCommand struct {
	prepare bool
	brandID string
	cycleID string
}

// parseCommissionReviewArgs recognizes only the two new CLI commands. Their
// arguments are validated before configuration or database access.
func parseCommissionReviewArgs(args []string) (commissionReviewCommand, bool, error) {
	if len(args) == 0 {
		return commissionReviewCommand{}, false, nil
	}
	switch args[0] {
	case "prepare-commission-history-review":
		if len(args) != 2 || args[1] != commissionReviewConfirmation {
			return commissionReviewCommand{}, true, errors.New("usage: platform prepare-commission-history-review --confirm=offline-commission-review")
		}
		return commissionReviewCommand{prepare: true}, true, nil
	case "commission-history-review":
		if len(args) != 3 || !canonicalLowerUUID.MatchString(args[1]) || !canonicalLowerUUID.MatchString(args[2]) {
			return commissionReviewCommand{}, true, errors.New("usage: platform commission-history-review <canonical-lowercase-brandUUID> <canonical-lowercase-cycleUUID>")
		}
		return commissionReviewCommand{brandID: args[1], cycleID: args[2]}, true, nil
	default:
		return commissionReviewCommand{}, false, nil
	}
}

func runCommissionReviewCommand(ctx context.Context, db *pgxpool.Pool, command commissionReviewCommand, stdout io.Writer) error {
	if command.prepare {
		if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
			return errors.New("commission history review preparation failed")
		}
		result := struct {
			Status     string `json:"status"`
			Checkpoint int    `json:"checkpoint"`
			Scope      string `json:"scope"`
		}{"prepared", 74, "read-only commission review only; not a full upgrade, money operation, or recovery"}
		if err := writeCommissionReviewJSON(stdout, result); err != nil {
			return errors.New("commission history review result unavailable")
		}
		return nil
	}

	report, err := commissionreview.Inspect(ctx, db, command.brandID, command.cycleID)
	if err != nil {
		if errors.Is(err, commissionreview.ErrCheckpoint) {
			return errors.New("commission history review requires explicit 0074 offline preparation")
		}
		if errors.Is(err, reporting.ErrAnalysisIntegrity) {
			return errors.New("commission history review source integrity failed; no report released")
		}
		if errors.Is(err, commissionreview.ErrNotFound) {
			return errors.New("commission history review cycle not found")
		}
		return errors.New("commission history review failed")
	}
	if err := writeCommissionReviewJSON(stdout, report); err != nil {
		return errors.New("commission history review result unavailable")
	}
	return nil
}

func writeCommissionReviewJSON(stdout io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	n, err := stdout.Write(encoded)
	if err == nil && n != len(encoded) {
		return io.ErrShortWrite
	}
	return err
}
