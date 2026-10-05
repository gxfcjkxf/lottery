// Package audit appends redacted business audit records in the caller's transaction.
package audit

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

type Record struct {
	BrandID, ActorType, ActorID, Action, ResourceType, ResourceID, Reason, RequestID, IP string
	Before, After                                                                        any
}

func Append(ctx context.Context, tx pgx.Tx, r Record) (string, error) {
	before, err := json.Marshal(r.Before)
	if err != nil {
		return "", err
	}
	after, err := json.Marshal(r.After)
	if err != nil {
		return "", err
	}
	id := ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,actor_id,action,resource_type,resource_id,reason,before_json,after_json,request_id,ip_address)
 VALUES($1,NULLIF($2,'')::uuid,$3,NULLIF($4,'')::uuid,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10,$11,NULLIF($12,''))`, id, r.BrandID, r.ActorType, r.ActorID, r.Action, r.ResourceType, r.ResourceID, r.Reason, before, after, r.RequestID, r.IP)
	return id, err
}
