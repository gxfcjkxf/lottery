package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/commissionreview"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const commissionProposalConfirmation = "--confirm=record-only-review"

var (
	commissionBearerPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	commissionDigestPattern      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	commissionMutationKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_:.-]{8,128}$`)
)

type commissionProposalCommand struct {
	mode       string
	brandID    string
	cycleID    string
	proposalID string
	version    int64
}

func parseCommissionProposalArgs(args []string) (commissionProposalCommand, bool, error) {
	if len(args) == 0 {
		return commissionProposalCommand{}, false, nil
	}
	usage := "usage: platform commission-recovery-source <brandUUID> <cycleUUID> | commission-recovery-propose <brandUUID> <cycleUUID> --confirm=record-only-review | commission-recovery-review <brandUUID> <proposalUUID> <positiveVersion> --confirm=record-only-review | commission-recovery-get <brandUUID> <proposalUUID>"
	bad := func() (commissionProposalCommand, bool, error) {
		return commissionProposalCommand{}, true, errors.New(usage)
	}
	command := commissionProposalCommand{}
	switch args[0] {
	case "commission-recovery-source":
		if !argCount(args, 3) || !CanonicalIDs(args[1], args[2]) {
			return bad()
		}
		command.mode, command.brandID, command.cycleID = "source", args[1], args[2]
	case "commission-recovery-propose":
		if !argCount(args, 4) || args[3] != commissionProposalConfirmation || !CanonicalIDs(args[1], args[2]) {
			return bad()
		}
		command.mode, command.brandID, command.cycleID = "propose", args[1], args[2]
	case "commission-recovery-review":
		if !argCount(args, 5) || args[4] != commissionProposalConfirmation || !CanonicalIDs(args[1], args[2]) {
			return bad()
		}
		version, err := strconv.ParseInt(args[3], 10, 64)
		if err != nil || version < 1 || version > 9007199254740991 || strconv.FormatInt(version, 10) != args[3] {
			return bad()
		}
		command.mode, command.brandID, command.proposalID, command.version = "review", args[1], args[2], version
	case "commission-recovery-get":
		if !argCount(args, 3) || !CanonicalIDs(args[1], args[2]) {
			return bad()
		}
		command.mode, command.brandID, command.proposalID = "get", args[1], args[2]
	default:
		return commissionProposalCommand{}, false, nil
	}
	return command, true, nil
}

// CanonicalIDs accepts only canonical lowercase UUID arguments.
func CanonicalIDs(values ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !canonicalLowerUUID.MatchString(value) {
			return false
		}
	}
	return true
}

func argCount(args []string, want int) bool { return len(args) == want }

type commissionProposalEnvironment struct {
	bearer string
	reason string
	digest string
	key    string
}

func validateCommissionProposalEnvironment(command commissionProposalCommand, env commissionProposalEnvironment) error {
	if !commissionBearerPattern.MatchString(env.bearer) {
		return errors.New("commission recovery bearer configuration unavailable")
	}
	if command.mode == "propose" || command.mode == "review" {
		if strings.TrimSpace(env.reason) == "" || len([]byte(env.reason)) > 500 {
			return errors.New("commission recovery reason configuration unavailable")
		}
		if !commissionDigestPattern.MatchString(env.digest) {
			return errors.New("commission recovery digest configuration unavailable")
		}
		if !commissionMutationKeyPattern.MatchString(env.key) {
			return errors.New("commission recovery mutation key configuration unavailable")
		}
	}
	return nil
}

func runCommissionProposalCommand(ctx context.Context, db *pgxpool.Pool, c config.Config, command commissionProposalCommand, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	env := commissionProposalEnvironmentFromOS()
	if err := validateCommissionProposalEnvironment(command, env); err != nil {
		return err
	}
	users, err := identity.New(db)
	if err != nil {
		return errors.New("commission recovery authentication unavailable")
	}
	adminID, err := users.AdminAuthenticate(ctx, env.bearer)
	if err != nil {
		return errors.New("commission recovery authentication unavailable")
	}
	actor, err := (adminsys.Store{DB: db}).Account(ctx, adminID)
	if err != nil || actor.Type != access.AccountAdmin {
		return errors.New("commission recovery authentication unavailable")
	}

	if command.mode == "source" || command.mode == "get" {
		return runCommissionProposalRead(ctx, db, users, env.bearer, actor, command, stdout)
	}
	key, err := loadKey(c)
	if err != nil {
		return errors.New("commission recovery mutation configuration unavailable")
	}
	engine, err := mutation.New(db, key)
	if err != nil {
		return errors.New("commission recovery mutation configuration unavailable")
	}
	return runCommissionProposalWrite(ctx, db, users, engine, env.bearer, env, actor, command, stdout)
}

func commissionProposalEnvironmentFromOS() commissionProposalEnvironment {
	return commissionProposalEnvironment{
		bearer: getenvRaw("LOTTERY_COMMISSION_RECOVERY_BEARER"),
		reason: getenvRaw("LOTTERY_COMMISSION_RECOVERY_REASON"),
		digest: getenvRaw("LOTTERY_COMMISSION_RECOVERY_DIGEST"),
		key:    getenvRaw("LOTTERY_COMMISSION_RECOVERY_KEY"),
	}
}

func getenvRaw(name string) string { return os.Getenv(name) }

func runCommissionProposalRead(ctx context.Context, db *pgxpool.Pool, users *identity.Store, bearer string, actor access.Account, command commissionProposalCommand, stdout io.Writer) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return errors.New("commission recovery read failed")
	}
	defer tx.Rollback(ctx)
	if err = authenticateCommissionProposalTx(ctx, tx, users, bearer, actor); err != nil {
		return errors.New("commission recovery authentication unavailable")
	}
	if _, err = commissionreview.AuthorizeProposalTx(ctx, tx, actor, command.brandID, "view"); err != nil {
		return errors.New("commission recovery access denied")
	}
	var result any
	if command.mode == "source" {
		var status string
		if err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, command.brandID).Scan(&status); err != nil || status == "disabled" {
			return errors.New("commission recovery source unavailable")
		}
		var cycleID string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM commission_cycles WHERE brand_id=$1 AND id=$2 FOR SHARE NOWAIT`, command.brandID, command.cycleID).Scan(&cycleID); err != nil {
			return errors.New("commission recovery source unavailable")
		}
		snapshot, snapshotErr := commissionreview.SnapshotTx(ctx, tx, command.brandID, command.cycleID)
		if snapshotErr != nil {
			return errors.New("commission recovery source unavailable")
		}
		result = snapshot
	} else {
		proposal, readErr := commissionreview.ReadProposalTx(ctx, tx, actor, command.brandID, command.proposalID)
		if readErr != nil {
			return errors.New("commission recovery proposal unavailable")
		}
		result = proposal
	}
	if err = authenticateCommissionProposalTx(ctx, tx, users, bearer, actor); err != nil {
		return errors.New("commission recovery authentication unavailable")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("commission recovery read failed")
	}
	if err = writeCommissionReviewJSON(stdout, result); err != nil {
		return errors.New("commission recovery result unavailable")
	}
	return nil
}

func runCommissionProposalWrite(ctx context.Context, db *pgxpool.Pool, users *identity.Store, engine *mutation.Engine, bearer string, env commissionProposalEnvironment, actor access.Account, command commissionProposalCommand, stdout io.Writer) error {
	operation := "commission.recovery.propose"
	input := any(commissionreview.ProposalInput{CycleID: command.cycleID, SourceDigest: env.digest, Reason: env.reason})
	if command.mode == "review" {
		operation = "commission.recovery.review"
		input = commissionreview.ReviewInput{Version: command.version, SourceDigest: env.digest, Reason: env.reason}
	}
	canonical, err := json.Marshal(struct {
		Command    string `json:"command"`
		BrandID    string `json:"brand_id"`
		ProposalID string `json:"proposal_id"`
		Input      any    `json:"input"`
	}{command.mode, command.brandID, command.proposalID, input})
	if err != nil {
		return errors.New("commission recovery mutation failed")
	}
	actorKey := "admin:" + actor.ID + ":" + strconv.FormatInt(actor.Version, 10)
	check := func(checkCtx context.Context, tx pgx.Tx) error {
		if err := authenticateCommissionProposalTx(checkCtx, tx, users, bearer, actor); err != nil {
			return err
		}
		_, err := commissionreview.AuthorizeProposalTx(checkCtx, tx, actor, command.brandID, command.mode)
		return err
	}
	run := func(runCtx context.Context, tx pgx.Tx) (mutation.Result, error) {
		if _, err := commissionreview.AuthorizeProposalTx(runCtx, tx, actor, command.brandID, command.mode); err != nil {
			return mutation.Result{}, err
		}
		meta := points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
		if command.mode == "propose" {
			proposal, err := commissionreview.ProposeTx(runCtx, tx, actor, command.brandID, input.(commissionreview.ProposalInput), meta)
			if err != nil {
				return mutation.Result{}, err
			}
			if err = authenticateCommissionProposalTx(runCtx, tx, users, bearer, actor); err != nil {
				return mutation.Result{}, err
			}
			return mutation.OK(201, proposal), nil
		}
		proposal, err := commissionreview.ReviewTx(runCtx, tx, actor, command.brandID, command.proposalID, input.(commissionreview.ReviewInput), meta)
		if err != nil {
			return mutation.Result{}, err
		}
		if err = authenticateCommissionProposalTx(runCtx, tx, users, bearer, actor); err != nil {
			return mutation.Result{}, err
		}
		return mutation.OK(200, proposal), nil
	}
	result, err := engine.ExecuteChecked(ctx, command.brandID, actorKey, operation, env.key, engine.Fingerprint(string(canonical)), check, run)
	if err != nil {
		return errors.New("commission recovery mutation failed")
	}
	if result.Status < 200 || result.Status >= 300 || result.Error != nil {
		return errors.New("commission recovery mutation rejected")
	}
	var data json.RawMessage
	if len(result.Data) == 0 || json.Unmarshal(result.Data, &data) != nil {
		return errors.New("commission recovery result unavailable")
	}
	if err = writeCommissionReviewJSON(stdout, data); err != nil {
		return errors.New("commission recovery result unavailable")
	}
	return nil
}

func authenticateCommissionProposalTx(ctx context.Context, tx pgx.Tx, users *identity.Store, bearer string, initial access.Account) error {
	actual, err := (adminsys.Store{}).LockAdminAccess(ctx, tx, initial.ID, false)
	if err != nil || actual.Version != initial.Version {
		return identity.ErrSession
	}
	authenticatedID, err := users.AdminAuthenticateTx(ctx, tx, bearer)
	if err != nil || authenticatedID != actual.ID {
		return identity.ErrSession
	}
	return nil
}
