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
	"commission.corrected",
	"commission.paid",
	"draw.result.corrected",
	"draw.result.published",
	"member.joined",
	"recharge.confirmed",
	"reward.order.granted",
	"reward.order.revoked",
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
	if strings.HasPrefix(key, "draw.result.") {
		pointsEN, pointsZH = "Historical result ID {resource_id}.", "历史结果 ID：{resource_id}。"
	} else if key == "member.joined" {
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

func TestEmbeddedTemplateDefaultsCoverEverySortedKey(t *testing.T) {
	var defaults map[string]Content
	if err := json.Unmarshal(templateDefaultsFixture, &defaults); err != nil {
		t.Fatal(err)
	}
	if len(defaults) != len(expectedTemplateKeys) {
		t.Fatalf("embedded defaults=%d, keys=%d", len(defaults), len(expectedTemplateKeys))
	}
	for _, key := range expectedTemplateKeys {
		content, ok := defaults[key]
		if !ok {
			t.Errorf("embedded defaults missing %q", key)
			continue
		}
		if err := ValidateContent(key, content); err != nil {
			t.Errorf("embedded default %q: %v", key, err)
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

func TestDrawTemplateRequiresHistoricalResultIDAndForbidsPoints(t *testing.T) {
	for _, key := range []string{"draw.result.published", "draw.result.corrected"} {
		content := validTemplateContent(key)
		if err := ValidateContent(key, content); err != nil {
			t.Errorf("ValidateContent(%q): %v", key, err)
		}
		for _, edit := range []struct {
			name  string
			apply func(*Content)
		}{
			{"missing English resource_id", func(c *Content) { c.En.Body = "Historical result." }},
			{"missing Chinese resource_id", func(c *Content) { c.ZhCN.Body = "历史结果。" }},
			{"points English body", func(c *Content) { c.En.Body = "Historical result {resource_id}: {points}." }},
			{"points Chinese body", func(c *Content) { c.ZhCN.Body = "历史结果 {resource_id}：{points}。" }},
			{"points English title", func(c *Content) { c.En.Title = "Result {points}" }},
			{"points Chinese title", func(c *Content) { c.ZhCN.Title = "结果 {points}" }},
		} {
			t.Run(key+"/"+edit.name, func(t *testing.T) {
				invalid := content
				edit.apply(&invalid)
				if err := ValidateContent(key, invalid); err == nil {
					t.Fatal("expected validation error")
				}
			})
		}
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
	a := access.Account{ID: admin, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopePlatform}}}}}
	return Service{DB: db}, a
}

func templateMeta(a access.Account) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
}

func updateTemplate(t *testing.T, s Service, a access.Account, key string, version int64, content Content) (Template, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.UpdateTemplate(ctx, tx, key, a, UpdateInput{Version: version, Content: content, Reason: " template test update "}, templateMeta(a))
	if err != nil {
		_ = tx.Rollback(ctx)
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Template{}, err
	}
	return out, nil
}

func TestPlatformTemplatesDefaultsAndHistory(t *testing.T) {
	s, _ := templateFixture(t)
	ctx := context.Background()
	items, err := s.Templates(ctx)
	if err != nil || len(items) != len(expectedTemplateKeys) {
		t.Fatalf("Templates() len=%d err=%v", len(items), err)
	}
	for i, item := range items {
		if item.Key != expectedTemplateKeys[i] || item.Version != 1 || item.AuditLogID != nil {
			t.Fatalf("unexpected initial template: %+v", item)
		}
		history, e := s.TemplateHistory(ctx, item.Key, 100, 0)
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
	locked, err := s.TemplateTx(ctx, tx, "member.joined", true)
	if err != nil || locked.Version != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplateTx(lock=true)=%+v err=%v", locked, err)
	}
	txItems, err := s.TemplatesTx(ctx, tx)
	if err != nil || len(txItems) != len(expectedTemplateKeys) {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplatesTx() len=%d err=%v", len(txItems), err)
	}
	txHistory, err := s.TemplateHistoryTx(ctx, tx, "member.joined", 10, 0)
	if err != nil || len(txHistory) != 1 || txHistory[0].Version != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("TemplateHistoryTx()=%+v err=%v", txHistory, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Template(ctx, "member.joined"); err != nil {
		t.Fatal(err)
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
	updated, err := updateTemplate(t, s, a, "recharge.confirmed", 1, content)
	if err != nil || updated.Version != 2 || updated.AuditLogID == nil || *updated.AuditLogID == "" {
		t.Fatalf("UpdateTemplate()=%+v err=%v", updated, err)
	}
	history, err := s.TemplateHistory(ctx, "recharge.confirmed", 100, 0)
	if err != nil || len(history) != 2 || history[0].Version != 2 || history[0].ChangedBy == nil || *history[0].ChangedBy != a.ID || history[0].AuditLogID == nil || *history[0].AuditLogID != *updated.AuditLogID {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	var fundsAfter int64
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&fundsAfter); err != nil || fundsAfter != fundsBefore {
		t.Fatalf("template update changed funds ledger: before=%d after=%d err=%v", fundsBefore, fundsAfter, err)
	}
	foreign, err := s.Template(ctx, "recharge.confirmed")
	if err != nil || foreign.Version != 2 || !reflect.DeepEqual(foreign.Content, updated.Content) {
		t.Fatalf("global template read differs: %+v %v", foreign, err)
	}
	if _, err = s.TemplateHistory(ctx, "recharge.confirmed", 100, 0); err != nil {
		t.Fatal(err)
	}
	var auditAction string
	if err = s.DB.QueryRow(ctx, `SELECT action FROM audit_logs WHERE id=$1`, *updated.AuditLogID).Scan(&auditAction); err != nil || auditAction != "notification.template.update" {
		t.Fatalf("audit action=%q err=%v", auditAction, err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE platform_notification_template_revisions SET reason='rewritten evidence' WHERE template_key=$1 AND version=2`, "recharge.confirmed"); err == nil {
		t.Fatal("revision evidence was mutable")
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM platform_notification_template_revisions WHERE template_key=$1 AND version=2`, "recharge.confirmed"); err == nil {
		t.Fatal("revision evidence was deletable")
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM platform_notification_templates WHERE template_key=$1`, "recharge.confirmed"); err == nil {
		t.Fatal("current template was deletable")
	}
	if _, err = updateTemplate(t, s, a, "recharge.confirmed", 1, content); !errors.Is(err, ErrTemplateVersion) {
		t.Fatalf("stale template version error=%v", err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, templateBrand); err != nil {
		t.Fatal(err)
	}
	if _, err = updateTemplate(t, s, a, "recharge.confirmed", 2, content); err != nil {
		t.Fatalf("global update unexpectedly depends on brand status=%v", err)
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
	_, err = s.UpdateTemplate(ctx, tx, "member.joined", a, UpdateInput{Version: 1, Content: validTemplateContent("member.joined"), Reason: "audit fails"}, templateMeta(a))
	if err == nil {
		t.Fatal("audit failure ignored")
	}
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		t.Fatal(rollbackErr)
	}
	current, err := s.Template(ctx, "member.joined")
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
			_, err = s.UpdateTemplate(ctx, tx, "member.joined", a, UpdateInput{Version: 1, Content: content, Reason: "concurrent template update"}, templateMeta(a))
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
	current, err := s.Template(ctx, "member.joined")
	if err != nil || success != 1 || conflict != 1 || current.Version != 2 {
		t.Fatalf("success=%d conflict=%d current=%+v err=%v", success, conflict, current, err)
	}
}

func TestAllowedTemplateRequiresExactGrantAndScope(t *testing.T) {
	brand := access.Account{Type: access.AccountAdmin, BrandIDs: []string{templateBrand}, Roles: []access.Role{{BrandID: templateBrand, Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand}, {Resource: "notification_template", Action: "view", Scope: access.ScopeBrand}}}}}
	if AllowedTemplate(brand, "write") || AllowedTemplate(brand, "view") {
		t.Fatal("brand permission authorized global configuration")
	}
	platform := access.Account{Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "notification_template", Action: "view", Scope: access.ScopePlatform}}}}}
	if !AllowedTemplate(platform, "view") || AllowedTemplate(platform, "write") {
		t.Fatal("explicit platform grants not respected")
	}
	platform.Roles[0].Permissions = append(platform.Roles[0].Permissions, access.Permission{Resource: "notification_template", Action: "write", Scope: access.ScopePlatform})
	if !AllowedTemplate(platform, "write") {
		t.Fatal("explicit platform writer denied")
	}
}

func TestSystemTemplateReferencesAreIdentityOnly(t *testing.T) {
	content := Content{I18nKey: "member.joined", I18nVersion: 1}
	if err := ValidateContent("member.joined", content); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(content)
	if err != nil || string(raw) != `{"i18n_key":"member.joined","i18n_version":1}` {
		t.Fatalf("reference leaked copy: %s, %v", raw, err)
	}
	for _, bad := range []Content{
		{I18nKey: "recharge.confirmed", I18nVersion: 1},
		{I18nKey: "member.joined", I18nVersion: 2},
		{I18nKey: "member.joined", I18nVersion: 1, En: Copy{Title: "Extra", Body: "Extra"}},
	} {
		if ValidateContent("member.joined", bad) == nil {
			t.Fatalf("accepted invalid reference: %+v", bad)
		}
	}
}

func TestTemplateUnicodeCharacterLimitsPersist(t *testing.T) {
	s, actor := templateFixture(t)
	ctx := context.Background()
	content := validTemplateContent("recharge.confirmed")
	content.En = Copy{Title: strings.Repeat("😀", 120), Body: strings.Repeat("😀", 1192) + "{points}"}
	content.ZhCN = Copy{Title: strings.Repeat("文", 120), Body: strings.Repeat("文", 1192) + "{points}"}
	reason := strings.Repeat("😀", 500)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	updated, err := s.UpdateTemplate(ctx, tx, "recharge.confirmed", actor, UpdateInput{Version: 1, Content: content, Reason: reason}, templateMeta(actor))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Template(ctx, "recharge.confirmed")
	if err != nil || !reflect.DeepEqual(saved.Content, content) {
		t.Fatalf("Unicode template did not persist: %v", err)
	}
	var savedReason string
	if err = s.DB.QueryRow(ctx, `SELECT reason FROM platform_notification_template_revisions WHERE template_key='recharge.confirmed' AND version=$1`, updated.Version).Scan(&savedReason); err != nil || savedReason != reason {
		t.Fatalf("Unicode reason did not persist: %v", err)
	}
	content.En.Title += "😀"
	if ValidateContent("recharge.confirmed", content) == nil {
		t.Fatal("121-character title accepted")
	}
	content.En.Title = strings.Repeat("😀", 120)
	content.En.Body += "😀"
	if ValidateContent("recharge.confirmed", content) == nil {
		t.Fatal("1201-character body accepted")
	}
}
