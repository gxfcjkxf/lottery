package cash

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func TestConfigDisableResetAndInheritance(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	meta := points.Metadata{ActorType: "system"}
	b, err := ReadConfig(ctx, tx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	b.Rules[1].Enabled = false
	b.Rules[1].DepositMin = ""
	b.Rules[1].Mode = "amount"
	b.Rules[1].Threshold = ""
	b, err = SaveConfig(ctx, tx, f.brand, b, meta)
	if err != nil {
		t.Fatal(err)
	}
	r := b.Rules[1]
	if r.Enabled || !r.Inherit || r.DepositMin != "0" || r.Rate != "" || r.Mode != "count" || r.Threshold != "1" {
		t.Fatalf("not reset: %+v", r)
	}
	b.Rules[1].Enabled = true
	b, err = SaveConfig(ctx, tx, f.brand, b, meta)
	if err != nil {
		t.Fatal(err)
	}
	if b.Rules[1].DepositMin != "1" {
		t.Fatalf("not inherited: %+v", b.Rules[1])
	}
	p, err := ReadConfig(ctx, tx, "platform")
	if err != nil {
		t.Fatal(err)
	}
	p.Rules[1].DepositMin = "5"
	p, err = SaveConfig(ctx, tx, "platform", p, meta)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := ReadBrandConfig(ctx, tx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Rules[1].DepositMin != "5" || len(effective.PlatformRules) != 4 {
		t.Fatalf("platform update not applied: %+v", effective)
	}
	_, br, err := rules(ctx, tx, f.brand, "USD")
	if err != nil || br.DepositMin != "5" {
		t.Fatalf("transaction rule: %+v %v", br, err)
	}
	b.Rules[1].Inherit = false
	b.Rules[1].DepositMin = "6"
	b, err = SaveConfig(ctx, tx, f.brand, b, meta)
	if err != nil {
		t.Fatal(err)
	}
	effective, err = ReadBrandConfig(ctx, tx, f.brand)
	if err != nil || effective.Rules[1].DepositMin != "6" {
		t.Fatalf("custom rule: %+v %v", effective, err)
	}
	p.Rules[1].Enabled = false
	p, err = SaveConfig(ctx, tx, "platform", p, meta)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rules[1].DepositMin != "0" || p.Rules[1].Rate != "" {
		t.Fatalf("platform not reset: %+v", p.Rules[1])
	}
	if _, _, err = rules(ctx, tx, f.brand, "USD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("platform gate: %v", err)
	}
	b.Rules[1].Inherit = true
	if _, err = SaveConfig(ctx, tx, f.brand, b, meta); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled platform save: %v", err)
	}
}
