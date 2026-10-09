package main

import (
	"log"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func drawNotificationExamples() map[string]any {
	const (
		id = "11111111-1111-4111-8111-111111111111"
	)
	previousID := "22222222-2222-4222-8222-222222222222"
	drawnAt := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	publishedContent := notification.Content{
		En:   notification.Copy{Title: "Draw result published", Body: "Historical result ID {resource_id}: the result published for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice."},
		ZhCN: notification.Copy{Title: "开奖结果已公布", Body: "历史开奖结果 ID：{resource_id}，表示本期已公布的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。"},
	}
	correctedContent := notification.Content{
		En:   notification.Copy{Title: "Draw result corrected", Body: "Historical result ID {resource_id}: the corrected result for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice."},
		ZhCN: notification.Copy{Title: "开奖结果已更正", Body: "历史开奖结果 ID：{resource_id}，表示本期已更正的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。"},
	}
	if err := notification.ValidateContent("draw.result.published", publishedContent); err != nil {
		log.Fatal(err)
	}
	if err := notification.ValidateContent("draw.result.corrected", correctedContent); err != nil {
		log.Fatal(err)
	}
	return map[string]any{
		"LotteryNotificationDrawPublished": notification.Item{
			ID: id, BrandID: id, MemberID: id, EventType: "draw.result.published", TemplateKey: "draw.result.published",
			TemplateVersion: 1, Content: &publishedContent,
			Payload: notification.Payload{ResourceID: id, Points: nil, Draw: &notification.DrawNotificationPayload{
				GameID: id, PeriodID: id, PeriodNo: "20261007001",
				Result: rules.Draw{Regular: []int{0, 14, 49}, Special: []int{}, Digits: []int{}}, DrawnAt: drawnAt,
			}}, CreatedAt: drawnAt,
		},
		"LotteryNotificationDrawCorrected": notification.Item{
			ID: id, BrandID: id, MemberID: id, EventType: "draw.result.corrected", TemplateKey: "draw.result.corrected",
			TemplateVersion: 1, Content: &correctedContent,
			Payload: notification.Payload{ResourceID: id, Points: nil, Draw: &notification.DrawNotificationPayload{
				GameID: id, PeriodID: id, PeriodNo: "20261007001",
				Result: rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{1, 2, 3}}, DrawnAt: drawnAt, PreviousDrawID: &previousID,
			}}, CreatedAt: drawnAt,
		},
	}
}
