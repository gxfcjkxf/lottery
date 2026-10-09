package main

import (
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/httpapi"
	"time"
)

func auditExportExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	brand := "22222222-2222-4222-8222-222222222222"
	createdAt := time.Date(2026, 10, 8, 12, 34, 56, 123456789, time.UTC)
	item := httpapi.AdminAuditRecord{
		ID: id, BrandID: &brand, Action: "user.write", ActorType: "admin", ActorID: id,
		ResourceType: "member", ResourceID: id, Reason: "contract example", RequestID: "audit-contract-example",
		CreatedAt: createdAt, IP: "192.0.2.1", Before: json.RawMessage(`{"status":"normal"}`),
		After: json.RawMessage(`{"status":"frozen"}`),
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		panic(err)
	}
	var legacy map[string]any
	if err = json.Unmarshal(encoded, &legacy); err != nil {
		panic(err)
	}
	delete(legacy, "brand_id") // Previous handler's map had no brand_id field.
	return map[string]any{
		"AdminAuditRecord":       item,
		"AdminAuditRecordLegacy": legacy,
		"AdminAuditList":         map[string]any{"items": []httpapi.AdminAuditRecord{item}},
	}
}
