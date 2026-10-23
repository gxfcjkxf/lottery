package betting

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFrozenRetiredSourcesStillPublishAndSettleOriginalOrders(t *testing.T) {
	for _, sourceType := range []string{"interval_json", "hk_marksix_6hch"} {
		t.Run(sourceType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				now := time.Now()
				if sourceType == "interval_json" {
					json.NewEncoder(w).Encode(map[string]any{"period_no": "historical-1", "drawn_at": now, "result": rules.Draw{Digits: []int{1, 2, 9}}})
					return
				}
				hk := now.In(time.FixedZone("Hong Kong", 8*60*60))
				json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": map[string]any{"businessCode": 0, "data": map[string]any{"preDrawIssue": hk.Year()*1000 + 1, "drawIssue": hk.Year()*1000 + 2, "preDrawTime": hk.Format("2006-01-02 15:04:05"), "drawTime": hk.Add(time.Hour).Format("2006-01-02 15:04:05"), "serverTime": hk.Format("2006-01-02 15:04:05"), "type": 8, "lotCode": 10048, "preDrawCode": "1,2,3,4,5,6,7", "color": []int{1, 1, 1, 1, 1, 1, 1}, "chineseZodiac": []int{7, 7, 7, 7, 7, 7, 7}, "fiveElements": []int{3, 3, 3, 3, 3, 3, 3}}}})
			}))
			defer server.Close()
			anchor := time.Now().Add(time.Hour)
			number := "000001"
			interval := 120
			plan := platformlottery.AutomationPlan{SourceType: sourceType, SourceURL: server.URL, AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval}
			kind := "canada_niuniu"
			want := points.Amount(810)
			if sourceType == "hk_marksix_6hch" {
				kind = "mark_six"
				want = 1470
				plan.AnchorDrawAt = nil
				plan.AnchorPeriodNo = nil
				plan.IntervalSeconds = nil
				plan.SourceURL += "/smallSix/findSmallSixInfo.do"
			}
			f, s, actor, game, period, orderID := historicalPurchase(t, kind, plan)
			ctx := context.Background()
			// Later configuration is distinct; the unpaid period keeps version 1.
			mapping := platformlottery.FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Digits: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/digits"}, Count: 3}}
			if kind == "mark_six" {
				mapping.Digits = nil
				mapping.Regular = &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/regular"}, Count: 6}
				mapping.Special = &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/result/special"}, Count: 1}
			}
			raw, _ := json.Marshal(mapping)
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `INSERT INTO platform_automation_versions(game_id,version,enabled,source_type,mapping,anchor_draw_at,anchor_period_no,interval_seconds,close_before_seconds,source_url,credential_ref,created_by) VALUES($1,2,false,'mapped',$2,$3,'900001',120,0,'https://new.example/feed','',$4)`, game.ID, raw, anchor, actor.ID); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `UPDATE platform_automation_settings SET version=2 WHERE game_id=$1`, game.ID)
				return err
			})
			time.Sleep(time.Until(period.DrawAt.Add(time.Second)))
			sources := platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()}
			var wg sync.WaitGroup
			counts := make(chan int, 2)
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					n, err := s.CollectAutomaticDraws(ctx, sources)
					if err != nil {
						t.Error(err)
					}
					counts <- n
				}()
			}
			wg.Wait()
			close(counts)
			published := 0
			for n := range counts {
				published += n
			}
			if published != 1 {
				t.Fatal("duplicate source publication", published)
			}
			settle := f.service.ProcessCanadaSettlements
			if kind == "mark_six" {
				settle = f.service.ProcessMarkSixSettlements
			}
			if n, err := settle(ctx, 10); err != nil || n != 1 {
				t.Fatal(n, err)
			}
			if n, err := settle(ctx, 10); err != nil || n != 0 {
				t.Fatal("duplicate settlement", n, err)
			}
			wallet, err := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
			if err != nil || wallet.AvailablePoints != want || wallet.SystemFrozenPoints != 0 {
				t.Fatal(wallet, err)
			}
			var consistent bool
			if err = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.brand, wallet.AccountID).Scan(&consistent); err != nil || !consistent {
				t.Fatal("fund evidence", err)
			}
			var version int
			var result []byte
			if err = f.db.QueryRow(ctx, `SELECT automation_version,result FROM platform_game_periods WHERE id=$1`, period.ID).Scan(&version, &result); err != nil || version != 1 || len(result) == 0 {
				t.Fatal("frozen period", version, string(result), err)
			}
			config, err := s.AutomationConfig(ctx, "", game.ID)
			if err != nil || len(config.Attempts) != 1 || config.Attempts[0].Code != "published" {
				t.Fatal("old attempts hidden after source switch", config.Attempts, err)
			}
			var auditCount int
			if err = f.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action IN('platform_game.automation.result','platform_game.hk.result')`, period.ID).Scan(&auditCount); err != nil || auditCount != 1 {
				t.Fatal("result audit", auditCount, err)
			}
			if sourceType == "hk_marksix_6hch" {
				check, err := s.ReadSourceCheck(ctx, game.ID, 1)
				if err != nil || check.LastResult.Attributes == nil || check.LastResult.Attributes.Waves["07"] != "red" {
					t.Fatal("source attributes", check, err)
				}
			}
			t.Log(fmt.Sprintf("%s: frozen version 1, one result audit, settled %s, wallet %d", sourceType, strings.TrimSpace(orderID), want))
		})
	}
}
