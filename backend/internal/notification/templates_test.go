package notification

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const templateBrand = "0199a000-0000-7000-8000-000000000001"
const templateOtherBrand = "0199a000-0000-7000-8000-000000000002"

var expectedTemplateKeys = []string{
	"bet.order.abnormal",
	"bet.order.cancelled",
	"bet.order.judged_cancelled",
	"bet.order.placed",
	"bet.order.prize_reversed",
	"bet.order.won",
	"commission.adjusted",
	"commission.paid",
	"member.joined",
	"recharge.confirmed",
	"withdrawal.order.cancelled",
	"withdrawal.order.failed",
	"withdrawal.order.paid",
	"withdrawal.order.processing",
	"withdrawal.order.rejected",
	"withdrawal.order.reviewing",
}

//go:embed template_defaults.json
var templateDefaultsFixture []byte

func validTemplateContent(key string) Content {
	pointsEN := "{points} points."
	pointsZH := "涉及 {points} 积分。"
	if key == "member.joined" {
		pointsEN, pointsZH = "Welcome aboard.", "欢迎加入。"
	}
	return Content{
		En:   Copy{Title: "Title", Body: pointsEN},
		ZhCN: Copy{Title: "标题", Body: pointsZH},
	}
}

func TestValidTemplateKeyIsExactAndSortedContract(t *testing.T) {
	for _, key := range expectedTemplateKeys {
		if !ValidTemplateKey(key) {
			t.Errorf("ValidTemplateKey(%q)=false", key)
		}
	}
	for _, key := range []string{"", "Member.joined", "notification.member.joined", "member.joined ", "bet.order.deleted"} {
		if ValidTemplateKey(key) {
			t.Errorf("ValidTemplateKey(%q)=true", key)
		}
	}
}

func TestValidateContentAcceptsSupportedPlaceholdersAndCanonicalText(t *testing.T) {
	for _, key := range templateKeys {
		content := validTemplateContent(key)
		if err := ValidateContent(key, content); err != nil {
			t.Errorf("ValidateContent(%q): %v", key, err)
		}
	}
	content := validTemplateContent("recharge.confirmed")
	content.En.Body = "Amount: {points}; reference {resource_id}."
	content.ZhCN.Body = "金额：{points}；参考 {resource_id}。"
	content.En.Title = "Receipt {resource_id}"
	if err := ValidateContent("recharge.confirmed", content); err != nil {
		t.Fatal(err)
	}
}

func TestValidTemplateVersionUsesSafeIntegerRange(t *testing.T) {
	for _, version := range []int64{1, 9007199254740991} {
		if !validTemplateVersion(version) {
			t.Errorf("validTemplateVersion(%d)=false", version)
		}
	}
	for _, version := range []int64{0, -1, 9007199254740992} {
		if validTemplateVersion(version) {
			t.Errorf("validTemplateVersion(%d)=true", version)
		}
	}
}

func TestValidateContentRejectsInvalidInputs(t *testing.T) {
	base := validTemplateContent("recharge.confirmed")
	tests := []struct {
		name string
		key  string
		edit func(*Content)
	}{
		{"unknown key", "other", nil},
		{"empty title", "recharge.confirmed", func(c *Content) { c.En.Title = "" }},
		{"trimmed title", "recharge.confirmed", func(c *Content) { c.En.Title = " title " }},
		{"empty body", "recharge.confirmed", func(c *Content) { c.ZhCN.Body = "\t " }},
		{"title too long", "recharge.confirmed", func(c *Content) { c.En.Title = strings.Repeat("x", 121) }},
		{"body too long", "recharge.confirmed", func(c *Content) { c.ZhCN.Body = strings.Repeat("界", 401) }},
		{"html bracket", "recharge.confirmed", func(c *Content) { c.En.Body = "<b>{points}</b>" }},
		{"https url", "recharge.confirmed", func(c *Content) { c.En.Body = "https://example.test {points}" }},
		{"javascript url", "recharge.confirmed", func(c *Content) { c.En.Body = "javascript:alert(1) {points}" }},
		{"data url", "recharge.confirmed", func(c *Content) { c.En.Body = "data:text/html,x {points}" }},
		{"www url", "recharge.confirmed", func(c *Content) { c.En.Body = "www.example.test {points}" }},
		{"unknown placeholder", "recharge.confirmed", func(c *Content) { c.En.Body = "{other} {points}" }},
		{"unclosed placeholder", "recharge.confirmed", func(c *Content) { c.En.Body = "{points {points}" }},
		{"stray closing brace", "recharge.confirmed", func(c *Content) { c.En.Body = "points} {points}" }},
		{"missing points", "recharge.confirmed", func(c *Content) { c.ZhCN.Body = "正文。" }},
		{"joined forbids points", "member.joined", func(c *Content) { c.En.Body = "Welcome {points}." }},
		{"joined title forbids points", "member.joined", func(c *Content) { c.En.Title = "Welcome {points}" }},
		{"title control", "recharge.confirmed", func(c *Content) { c.En.Title = "Title\n" }},
		{"body control", "recharge.confirmed", func(c *Content) { c.En.Body = "body\x01 {points}" }},
		{"invalid utf8", "recharge.confirmed", func(c *Content) { c.En.Title = string([]byte{0xff}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := base
			if tt.key == "member.joined" {
				content = validTemplateContent(tt.key)
			}
			if tt.edit != nil {
				tt.edit(&content)
			}
			if err := ValidateContent(tt.key, content); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if utf8.ValidString(string([]byte{0xff})) {
		t.Fatal("invalid UTF-8 test fixture unexpectedly valid")
	}
}

func templateFixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	admin := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, admin, "template_"+admin); err != nil {
		t.Fatal(err)
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{templateBrand}, Roles: []access.Role{{BrandID: templateBrand, Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand}}}}}
	return Service{DB: db}, a
}

func templateMeta(a access.Account) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
}

func updateTemplate(t *testing.T, s Service, a access.Account, brand, key string, version int64, content Content) (Template, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.UpdateTemplate(ctx, tx, brand, key, a, UpdateInput{Version: version, Content: content, Reason: " template test update "}, templateMeta(a))
	if err != nil {
		_ = tx.Rollback(ctx)
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Template{}, err
	}
	return out, nil
}

func TestTemplatesDefaultsHistoryAndBrandIsolation(t *testing.T) {
	s, _ := templateFixture(t)
	ctx := context.Background()
	items, err := s.Templates(ctx, templateBrand)
	if err != nil || len(items) != len(expectedTemplateKeys) {
		t.Fatalf("Templates() len=%d err=%v", len(items), err)
	}
	for i, item := range items {
		if item.Key != expectedTemplateKeys[i] || item.BrandID != templateBrand || item.Version != 1 || item.AuditLogID != nil {
			t.Fatalf("unexpected initial template: %+v", item)
		}
		history, e := s.TemplateHistory(ctx, templateBrand, item.Key, 100, 0)
		if e != nil || len(history) != 1 || history[0].Version != 1 || history[0].ChangedBy != nil || history[0].AuditLogID != nil {
			t.Fatalf("initial history for %s: %+v %v", item.Key, history, e)
		}
	}
	var defaults map[string]Content
	var raw []byte
	if err = s.DB.QueryRow(ctx, `SELECT notification_template_defaults()`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &defaults); err != nil || len(defaults) != len(expectedTemplateKeys) {
		t.Fatalf("defaults map len=%d err=%v", len(defaults), err)
	}
	var fixture map[string]Content
	if err = json.Unmarshal(templateDefaultsFixture, &fixture); err != nil || !reflect.DeepEqual(defaults, fixture) {
		t.Fatalf("database defaults differ from frontend defaults fixture: err=%v", err)
	}
	for _, item := range items {
		if expected, ok := defaults[item.Key]; !ok || !reflect.DeepEqual(item.Content, expected) {
			t.Fatalf("seeded content for %s differs from SQL defaults: %+v / %+v", item.Key, item.Content, expected)
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := s.TemplateTx(ctx, tx, templateBrand, "member.joined", true)
	if err != nil || locked.Version != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplateTx(lock=true)=%+v err=%v", locked, err)
	}
	txItems, err := s.TemplatesTx(ctx, tx, templateBrand)
	if err != nil || len(txItems) != len(expectedTemplateKeys) {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplatesTx() len=%d err=%v", len(txItems), err)
	}
	txHistory, err := s.TemplateHistoryTx(ctx, tx, templateBrand, "member.joined", 10, 0)
	if err != nil || len(txHistory) != 1 || txHistory[0].Version != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplateHistoryTx()=%+v err=%v", txHistory, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Template(ctx, templateOtherBrand, "member.joined"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Template(ctx, "0199a000-0000-7000-8000-000000000099", "member.joined"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing brand error=%v", err)
	}
}

func TestTemplateUpdateAuditHistoryAndNoFundsWrites(t *testing.T) {
	s, a := templateFixture(t)
	ctx := context.Background()
	var fundsBefore int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&fundsBefore); err != nil {
		t.Fatal(err)
	}
	content := validTemplateContent("recharge.confirmed")
	content.En = Copy{Title: "New recharge title", Body: "New points: {points}."}
	content.ZhCN = Copy{Title: "新充值标题", Body: "新积分：{points}。"}
	updated, err := updateTemplate(t, s, a, templateBrand, "recharge.confirmed", 1, content)
	if err != nil || updated.Version != 2 || updated.AuditLogID == nil || *updated.AuditLogID == "" {
		t.Fatalf("UpdateTemplate()=%+v err=%v", updated, err)
	}
	history, err := s.TemplateHistory(ctx, templateBrand, "recharge.confirmed", 100, 0)
	if err != nil || len(history) != 2 || history[0].Version != 2 || history[0].ChangedBy == nil || *history[0].ChangedBy != a.ID || history[0].AuditLogID == nil || *history[0].AuditLogID != *updated.AuditLogID {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	var fundsAfter int64
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&fundsAfter); err != nil || fundsAfter != fundsBefore {
		t.Fatalf("template update changed funds ledger: before=%d after=%d err=%v", fundsBefore, fundsAfter, err)
	}
	foreign, err := s.Template(ctx, templateOtherBrand, "recharge.confirmed")
	if err != nil || foreign.Version != 1 || reflect.DeepEqual(foreign.Content, updated.Content) {
		t.Fatalf("brand isolation failed: %+v %v", foreign, err)
	}
	if _, err = s.TemplateHistory(ctx, templateOtherBrand, "recharge.confirmed", 100, 0); err != nil {
		t.Fatal(err)
	}
	var auditAction string
	if err = s.DB.QueryRow(ctx, `SELECT action FROM audit_logs WHERE id=$1`, *updated.AuditLogID).Scan(&auditAction); err != nil || auditAction != "notification.template.update" {
		t.Fatalf("audit action=%q err=%v", auditAction, err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE notification_template_revisions SET reason='rewritten evidence' WHERE brand_id=$1 AND template_key=$2 AND version=2`, templateBrand, "recharge.confirmed"); err == nil {
		t.Fatal("revision evidence was mutable")
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM notification_template_revisions WHERE brand_id=$1 AND template_key=$2 AND version=2`, templateBrand, "recharge.confirmed"); err == nil {
		t.Fatal("revision evidence was deletable")
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM notification_templates WHERE brand_id=$1 AND template_key=$2`, templateBrand, "recharge.confirmed"); err == nil {
		t.Fatal("current template was deletable")
	}
	if _, err = updateTemplate(t, s, a, templateBrand, "recharge.confirmed", 1, content); !errors.Is(err, ErrTemplateVersion) {
		t.Fatalf("stale template version error=%v", err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, templateBrand); err != nil {
		t.Fatal(err)
	}
	if _, err = updateTemplate(t, s, a, templateBrand, "recharge.confirmed", 2, content); !errors.Is(err, ErrState) {
		t.Fatalf("disabled brand update error=%v", err)
	}
}

func TestTemplateUpdateAuditFailureRollsBackOnCallerRollback(t *testing.T) {
	s, a := templateFixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(ctx, `CREATE FUNCTION reject_template_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='notification.template.update' THEN RAISE EXCEPTION 'audit rejected'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_template_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_template_audit()`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateTemplate(ctx, tx, templateBrand, "member.joined", a, UpdateInput{Version: 1, Content: validTemplateContent("member.joined"), Reason: "audit fails"}, templateMeta(a))
	if err == nil {
		t.Fatal("audit failure ignored")
	}
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		t.Fatal(rollbackErr)
	}
	current, err := s.Template(ctx, templateBrand, "member.joined")
	if err != nil || current.Version != 1 {
		t.Fatalf("failed update persisted: %+v %v", current, err)
	}
}

func TestConcurrentTemplateUpdatesPermitExactlyOneVersion(t *testing.T) {
	s, a := templateFixture(t)
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			content := validTemplateContent("member.joined")
			content.En.Title = []string{"Welcome A", "Welcome B"}[i]
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			_, err = s.UpdateTemplate(ctx, tx, templateBrand, "member.joined", a, UpdateInput{Version: 1, Content: content, Reason: "concurrent template update"}, templateMeta(a))
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrTemplateVersion) {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	current, err := s.Template(ctx, templateBrand, "member.joined")
	if err != nil || success != 1 || conflict != 1 || current.Version != 2 {
		t.Fatalf("success=%d conflict=%d current=%+v err=%v", success, conflict, current, err)
	}
}

func TestAllowedTemplateRequiresExactGrantAndScope(t *testing.T) {
	brandActor := access.Account{Type: access.AccountAdmin, BrandIDs: []string{templateBrand}, Roles: []access.Role{{BrandID: templateBrand, Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand}, {Resource: "notification_template", Action: "view", Scope: access.ScopeBrand}}}}}
	if !AllowedTemplate(brandActor, templateBrand, "write") || !AllowedTemplate(brandActor, templateBrand, "view") {
		t.Fatal("exact brand permissions rejected")
	}
	if AllowedTemplate(brandActor, templateOtherBrand, "view") {
		t.Fatal("brand grant crossed tenant boundary")
	}
	super := brandActor
	super.SuperAdmin = true
	if AllowedTemplate(super, templateBrand, "write") {
		t.Fatal("super administrator may not write templates")
	}
	flat := access.Account{Type: access.AccountAdmin, BrandIDs: []string{templateBrand}}
	if AllowedTemplate(flat, templateBrand, "view") || AllowedTemplate(flat, templateBrand, "write") {
		t.Fatal("flat brand membership implicitly granted access")
	}
	platform := access.Account{Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "notification_template", Action: "view", Scope: access.ScopePlatform}}}}}
	if !AllowedTemplate(platform, templateBrand, "view") || AllowedTemplate(platform, templateBrand, "write") {
		t.Fatal("platform grant scope is incorrect")
	}
}
