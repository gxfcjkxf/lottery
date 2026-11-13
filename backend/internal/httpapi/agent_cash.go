package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/cash"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func agentCashContext(d Dependencies, v identity.AgentSession) string {
	return d.Mutations.Fingerprint("agent-cash-v1:" + v.BrandID + ":" + v.AccountID)
}
func agentCashRead(r *http.Request, d Dependencies, tx pgx.Tx, v identity.AgentSession, path string) (any, error) {
	if !cashReadParameters(r, path == "cash/orders") {
		return nil, cash.ErrInvalid
	}
	if path == "cash/orders" {
		if kind := r.URL.Query().Get("kind"); kind != "" && kind != "deposit" && kind != "withdrawal" && kind != "to_points" && kind != "from_points" {
			return nil, cash.ErrInvalid
		}
		limit, offset, _ := pageParams(r)
		return cash.List(r.Context(), tx, v.BrandID, v.AccountID, r.URL.Query().Get("kind"), limit, offset)
	}
	balances, e := cash.Balances(r.Context(), tx, v.BrandID, v.AccountID)
	if e != nil {
		return nil, e
	}
	platform, e := cash.ReadConfig(r.Context(), tx, "platform")
	if e != nil {
		return nil, e
	}
	brand, e := cash.ReadConfig(r.Context(), tx, v.BrandID)
	if e != nil {
		return nil, e
	}
	dest, e := cash.Destinations(r.Context(), tx, v.BrandID, v.AccountID)
	if e != nil {
		return nil, e
	}
	return map[string]any{"auto_usdt": true, "actor_context": agentCashContext(d, v), "brand_id": v.BrandID, "member_id": v.AccountID, "balances": balances, "destinations": dest, "platform": platform, "brand": brand}, nil
}
func agentCashWrite(w http.ResponseWriter, r *http.Request, d Dependencies, entryBrand string, initial identity.AgentSession, path string) {
	if !withdrawalNoQuery(w, r) {
		return
	}
	id, action := r.PathValue("id"), r.PathValue("action")
	if path == "cash/orders/{id}/{action}" && (!uuidPattern.MatchString(id) || (action != "submit" && action != "cancel")) {
		cashOutput(w, r, nil, cash.ErrInvalid)
		return
	}
	var in cash.Input
	var dest cash.Destination
	var deletion struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	var body any = &in
	if path == "cash/destinations" {
		body = &dest
	} else if path == "cash/destinations/delete" {
		body = &deletion
	}
	if !decodeBody(w, r, body) {
		return
	}
	raw, _ := json.Marshal(body)
	result, e := d.Mutations.ExecuteChecked(r.Context(), initial.BrandID, "agent:"+initial.AccountID, "agent."+path+":"+id+":"+action, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		v, e := d.Identity.AgentAuthenticateTx(ctx, tx, initial.BrandID, requestToken(r, agentCookieName(entryBrand)))
		if e != nil {
			return e
		}
		if v.AccountID != initial.AccountID || v.BrandID != initial.BrandID {
			return identity.ErrSession
		}
		expected := r.Header.Get("X-Cash-Actor-Context")
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(agentCashContext(d, v))) != 1 {
			return errCashActor
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		metadata := points.Metadata{ActorType: "agent", ActorID: initial.AccountID, RequestID: requestID(r), IP: meta(r).IP}
		s := cashService(d)
		var out any
		var err error
		if path == "cash/deposits" {
			out, err = s.DepositAgent(ctx, tx, initial.BrandID, initial.AccountID, in, metadata)
		} else if path == "cash/exchanges" {
			out, err = s.Exchange(ctx, tx, initial.BrandID, initial.AccountID, in, metadata)
		} else if path == "cash/withdrawals" {
			out, err = s.Withdraw(ctx, tx, initial.BrandID, initial.AccountID, in, metadata)
		} else if path == "cash/destinations/delete" {
			if !uuidPattern.MatchString(deletion.ID) {
				return cashResult(nil, cash.ErrInvalid)
			}
			out, err = cash.DeleteDestination(ctx, tx, initial.BrandID, initial.AccountID, deletion.ID, deletion.Version, metadata)
		} else if path == "cash/destinations" {
			if dest.ID != "" && !uuidPattern.MatchString(dest.ID) {
				return cashResult(nil, cash.ErrInvalid)
			}
			out, err = cash.SaveDestination(ctx, tx, initial.BrandID, initial.AccountID, dest, metadata)
		} else {
			out, err = s.Advance(ctx, tx, initial.BrandID, initial.AccountID, id, action, in, metadata)
		}
		if err == nil {
			_, err = d.Identity.AgentAuthenticateTx(ctx, tx, initial.BrandID, requestToken(r, agentCookieName(entryBrand)))
		}
		return cashResult(out, err)
	})
	if errors.Is(e, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "Agent session expired")
		return
	}
	if errors.Is(e, errCashActor) {
		result, e = cashResult(nil, e)
	}
	outputMutation(w, r, result, e)
}
