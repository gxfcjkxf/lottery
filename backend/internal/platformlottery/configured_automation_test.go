package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestConfiguredModelDrivesAutomationAndDrawValidation(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := Service{DB: db}
	actor := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actor.ID, "configured_auto_"+actor.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot rules.ConfiguredSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	// A configured 2+1 game with a 1..60 pool is neither dedicated game family.
	snapshot.Catalog.Model.RegularCount = 2
	snapshot.Catalog.Model.RegularPool.Max = 60
	snapshot.Catalog.Model.SpecialPool.Max = 60
	snapshot.Catalog.Plays[0].PushWhen = nil
	snapshot.Catalog.Plays[0].Tiers[0].When.Left.Scope.Positions = []int{1}
	raw, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := rules.RestoreConfiguredEngine(raw)
	if err != nil {
		t.Fatalf("custom snapshot: %v", err)
	}
	raw = engine.Snapshot()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	game, err := s.Create(ctx, tx, actor, GameInput{Code: "configured_auto", Name: "Configured model", Model: snapshot.Catalog.Model, Timezone: "UTC"}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().UTC().Add(time.Hour)
	number := "0001"
	interval := 120
	in := AutomationInput{AutomationPlan: AutomationPlan{Enabled: true, SourceType: "mapped", Mapping: &FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: FeedField{Path: "/period_no"}, DrawTime: FeedField{Path: "/drawn_at"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/result/regular"}, Count: 2}, Special: &FeedNumbers{FeedField: FeedField{Path: "/result/special"}, Count: 1}}, SourceURL: "https://draw.example.test/feed", AnchorDrawAt: &anchor, AnchorPeriodNo: &number, IntervalSeconds: &interval, OpenBeforeSeconds: &interval, CloseBeforeSeconds: 15}}
	if _, err = publishTestAutomation(ctx, s, tx, actor, game.ID, in, points.Metadata{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unpublished rules accepted: %v", err)
	}
	if _, err = s.SaveConfiguredRules(ctx, tx, actor, game.ID, ConfiguredRulesInput{Snapshot: raw}, points.Metadata{}); err != nil {
		t.Fatal(err)
	}
	hk := AutomationInput{AutomationPlan: AutomationPlan{Enabled: true, SourceType: "hk_marksix_6hch", SourceURL: "https://draw.example.test/smallSix/findSmallSixInfo.do", CloseBeforeSeconds: 15}}
	if _, err = publishTestAutomation(ctx, s, tx, actor, game.ID, hk, points.Metadata{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("source-specific model contract bypassed: %v", err)
	}
	if _, err = publishTestAutomation(ctx, s, tx, access.Account{}, game.ID, in, points.Metadata{}); !errors.Is(err, ErrDenied) {
		t.Fatalf("write grant bypassed: %v", err)
	}
	if _, err = publishTestAutomation(ctx, s, tx, actor, game.ID, in, points.Metadata{}); err != nil {
		t.Fatalf("configured automation rejected: %v", err)
	}
	period, err := s.CreatePeriod(ctx, tx, actor, game.ID, PeriodInput{PeriodNo: "manual", BetStartAt: time.Now().Add(-2 * time.Hour), BetEndAt: time.Now().Add(-time.Hour), DrawAt: time.Now().Add(-30 * time.Minute)}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	invalid := rules.Draw{Regular: []int{1, 7}, Special: []int{2}}
	invalid.Regular = []int{1, 7, 8}
	if _, err = s.Publish(ctx, tx, actor, period.ID, invalid, "", points.Metadata{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong configured count accepted: %v", err)
	}
	invalid = rules.Draw{Regular: []int{1, 7}, Special: []int{2}}
	invalid.Special = []int{61}
	if _, err = s.Publish(ctx, tx, actor, period.ID, invalid, "", points.Metadata{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("outside configured pool accepted: %v", err)
	}
	if _, err = s.Publish(ctx, tx, actor, period.ID, rules.Draw{Regular: []int{1, 7}, Special: []int{2}}, "", points.Metadata{}); err != nil {
		t.Fatalf("configured draw rejected: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if count, err := s.GenerateAutomaticPeriods(ctx); err != nil || count != 2 {
		t.Fatalf("configured automatic periods: count=%d err=%v", count, err)
	}
}

func TestUnpublishedGameCanDisableMappedSource(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := Service{DB: db}
	actor := systemAutomationActor(ids.New())
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actor.ID, "source_disable_"+actor.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	game, err := s.Create(ctx, tx, actor, GameInput{Code: "disabled_source", Name: "Disabled source", Timezone: "UTC", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	opening := 60
	plan := AutomationPlan{SourceType: "mapped", Enabled: false, OpenBeforeSeconds: &opening, SourceURL: "https://draw.example.test/feed", Mapping: &FeedMapping{Format: "json", Schedule: "source", TimeFormat: "rfc3339", Period: FeedField{Path: "/issue"}, DrawTime: FeedField{Path: "/time"}, Digits: &FeedNumbers{FeedField: FeedField{Path: "/digits"}, Count: 3}}}
	source, err := s.SaveDrawSource(ctx, tx, actor, game.ID, DrawSourceInput{DrawSource: DrawSource{Name: "Disabled", Plan: plan}}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateDrawSource(ctx, tx, actor, game.ID, source.ID, DrawSourceActivation{Revision: 1}, points.Metadata{}); err != nil {
		t.Fatal("disable must not require rules", err)
	}
	source.Plan.Enabled = true
	source, err = s.SaveDrawSource(ctx, tx, actor, game.ID, DrawSourceInput{DrawSource: source}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateDrawSource(ctx, tx, actor, game.ID, source.ID, DrawSourceActivation{Revision: source.Revision, Version: 1}, points.Metadata{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("enable without rules", err)
	}
}
