package notification

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func validateDrawEvent(ctx context.Context, tx pgx.Tx, brand, kind, aggregate string, raw []byte) (string, Payload, error) {
	if !exactCommissionJSONKeys(raw, "member_id", "resource_id", "publication_id", "recipient_id") || !uuid.MatchString(brand) || !uuid.MatchString(aggregate) {
		return "", Payload{}, ErrInvalid
	}
	var in struct {
		MemberID      string `json:"member_id"`
		ResourceID    string `json:"resource_id"`
		PublicationID string `json:"publication_id"`
		RecipientID   string `json:"recipient_id"`
	}
	if json.Unmarshal(raw, &in) != nil || !uuid.MatchString(in.MemberID) || !uuid.MatchString(in.ResourceID) || in.PublicationID != in.ResourceID || in.RecipientID != aggregate {
		return "", Payload{}, ErrInvalid
	}
	var facts DrawNotificationPayload
	var result []byte
	err := tx.QueryRow(ctx, `SELECT p.game_id::text,p.period_id::text,p.period_no,p.result,p.drawn_at,p.previous_draw_id::text
 FROM draw_notification_recipients r JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
 WHERE r.brand_id=$1 AND r.id=$3 AND valid_draw_notification_event($1,$2,$3,$4::jsonb)`, brand, kind, aggregate, raw).
		Scan(&facts.GameID, &facts.PeriodID, &facts.PeriodNo, &result, &facts.DrawnAt, &facts.PreviousDrawID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Payload{}, ErrInvalid
	}
	if err != nil {
		return "", Payload{}, err
	}
	if json.Unmarshal(result, &facts.Result) != nil {
		return "", Payload{}, ErrInvalid
	}
	facts.DrawnAt = facts.DrawnAt.UTC()
	return in.MemberID, Payload{ResourceID: in.ResourceID, Draw: &facts}, nil
}
