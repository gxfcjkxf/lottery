package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func complianceAdmission(ctx context.Context, tx pgx.Tx, brand, action, actorType, actor, member string, meta Metadata) (mutation.Result, error, bool) {
	err := compliance.AssessTx(ctx, tx, brand, action, compliance.GateSubject{ActorType: actorType, ActorID: actor, MemberID: member, RequestID: meta.RequestID, IP: meta.IP})
	if err == nil {
		return mutation.Result{}, nil, false
	}
	if result, ok := compliance.Rejection(err); ok {
		return result, nil, true
	}
	return mutation.Result{}, err, true
}
