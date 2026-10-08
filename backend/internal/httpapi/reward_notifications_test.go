package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
)

func TestRewardNotificationsHTTPPreserveOriginalReceiptsAndPrivateEvidence(t *testing.T) {
	f := rewardFixture(t)
	ctx := context.Background()
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand", "reward.revoke.brand", "reward.retry.brand")
	body := `{"member_id":"` + f.memberID + `","points":"40","reason":"private reward grant justification"}`
	original := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-notify-grant-key", f.token, managedBrand, f.root, body)
	mustStatus(t, original, 201)
	var order rewards.Order
	managedData(t, original, &order)
	frozen := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "reward-notify-freeze-key", f.token, managedBrand, map[string]string{"points": "40", "reason": "private hold reason"})
	mustStatus(t, frozen, 200)
	var entry points.Entry
	managedData(t, frozen, &entry)
	revokePath := rewardOrdersHTTPPath + "/" + order.ID + "/revoke"
	revokeBody := `{"version":1,"reason":"private pending reversal reason"}`
	pending := rewardRawCall(f.managementHTTP, "POST", revokePath, "reward-notify-revoke-key", f.token, managedBrand, f.root, revokeBody)
	mustStatus(t, pending, 200)
	var pendingOrder rewards.Order
	managedData(t, pending, &pendingOrder)
	if pendingOrder.State != "revocation_pending" || pendingOrder.Version != 2 {
		t.Fatal(pendingOrder)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/unfreeze", "reward-notify-unfreeze-key", f.token, managedBrand, map[string]string{"entry_id": entry.ID, "reason": "private hold release"}), 200)
	retryBody := `{"version":2,"reason":"private operator resumes original reversal"}`
	retryPath := rewardOrdersHTTPPath + "/" + order.ID + "/retry-revocation"
	done := rewardRawCall(f.managementHTTP, "POST", retryPath, "reward-notify-retry-key", f.token, managedBrand, f.root, retryBody)
	mustStatus(t, done, 200)
	var current rewards.Order
	managedData(t, done, &current)
	if current.State != "revoked" || current.Version != 3 {
		t.Fatal(current)
	}
	before := pointWallet(t, f)
	for _, call := range []struct {
		path, key, body string
		status          int
		receipt         rewards.Order
	}{
		{rewardOrdersHTTPPath, "reward-notify-grant-key", body, 201, order},
		{revokePath, "reward-notify-revoke-key", revokeBody, 200, pendingOrder},
		{retryPath, "reward-notify-retry-key", retryBody, 200, current},
	} {
		response := rewardRawCall(f.managementHTTP, "POST", call.path, call.key, f.token, managedBrand, f.root, call.body)
		mustStatus(t, response, call.status)
		var replay rewards.Order
		managedData(t, response, &replay)
		if !reflect.DeepEqual(replay, call.receipt) {
			t.Fatal("notification/retry replaced original business receipt")
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type LIKE 'reward.order.%'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("reward events=%d err=%v", count, err)
	}
	service := notification.Service{DB: f.pool}
	if _, err := service.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", "/api/v1/notifications?limit=100", "", f.userToken, managedBrand, nil)
	mustStatus(t, response, 200)
	var page notification.Page
	managedData(t, response, &page)
	ids := []string{}
	kinds := map[string]int{}
	for _, item := range page.Items {
		if !strings.HasPrefix(item.EventType, "reward.order.") {
			continue
		}
		kinds[item.EventType]++
		ids = append(ids, item.ID)
		if item.Content == nil || item.Payload.ResourceID != order.ID || item.Payload.Points == nil || *item.Payload.Points != "40" {
			t.Fatal(item)
		}
	}
	if !reflect.DeepEqual(kinds, map[string]int{"reward.order.granted": 1, "reward.order.revocation_pending": 1, "reward.order.revoked": 1}) {
		t.Fatal(kinds)
	}
	for _, secret := range []string{"private ", "audit_log_id", "ledger_entry_id", "action_id", "actor_id", "creation_xid"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("private notification evidence leaked: %s", secret)
		}
	}
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/notifications?limit=100", "", f.userToken, managedBrand, nil), 401)
	raw, _ := json.Marshal(map[string]any{"ids": ids})
	marked := f.call("POST", "/api/v1/notifications/read", "reward-notify-read-key", f.userToken, managedBrand, json.RawMessage(raw))
	mustStatus(t, marked, 200)
	var receipt notification.ReadReceipt
	managedData(t, marked, &receipt)
	if receipt.Changed != 3 {
		t.Fatal(receipt)
	}
	replay := f.call("POST", "/api/v1/notifications/read", "reward-notify-read-key", f.userToken, managedBrand, json.RawMessage(raw))
	mustStatus(t, replay, 200)
	var again notification.ReadReceipt
	managedData(t, replay, &again)
	if !reflect.DeepEqual(receipt, again) {
		t.Fatal("read receipt not idempotent")
	}
	if _, err := service.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if pointWallet(t, f) != before {
		t.Fatal("notification query/read/replay changed wallet")
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE event_type LIKE 'reward.order.%'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("duplicate notifications=%d err=%v", count, err)
	}
	if receipt.BrandID != managedBrand || receipt.MemberID != f.memberID {
		t.Fatal(fmt.Sprintf("wrong read receipt scope: %+v", receipt))
	}
}
