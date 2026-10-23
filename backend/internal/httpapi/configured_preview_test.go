package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func previewFixture(t *testing.T) configuredPreviewInput {
	t.Helper()
	// An operator-created play with an opaque ID unknown to the static catalogue.
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	return configuredPreviewInput{Snapshot: json.RawMessage(compact.Bytes()), Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 3, Multiplier: 1}}}
}

func TestConfiguredPreviewNewPlay(t *testing.T) {
	in := previewFixture(t)
	validated, err := configuredPreview(context.Background(), configuredPreviewInput{Snapshot: in.Snapshot, Requests: []rules.ConfiguredRequest{}})
	if err != nil || validated.(map[string]any)["quote"] != nil {
		t.Fatal(validated, err)
	}
	quoted, err := configuredPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if q := quoted.(map[string]any)["quote"].(rules.ConfiguredQuote); len(q.Lines) != 1 || q.BetPoints != 3 || q.Lines[0].Prices["hit"] != "2.5" {
		t.Fatal(q)
	}
	for _, tc := range []struct {
		n             int
		outcome       string
		prize, refund int64
	}{{7, "won", 8, 0}, {8, "lost", 0, 0}, {49, "push", 0, 3}} {
		in.Draw = &rules.Draw{Regular: []int{1, 3, tc.n, 4, 5, 6}, Special: []int{2}, Digits: []int{}}
		out, err := configuredPreview(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		r := out.(map[string]any)["result"].(rules.ConfiguredResult)
		if r.Lines[0].Outcome != tc.outcome || int64(r.PrizePoints) != tc.prize || int64(r.RefundPoints) != tc.refund {
			t.Fatal(r)
		}
	}
	in.Requests[0].Plate = "B"
	in.Draw = &rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}, Digits: []int{}}
	out, err := configuredPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if result := out.(map[string]any)["result"].(rules.ConfiguredResult); result.PrizePoints != 11 {
		t.Fatal(result)
	}
}

func TestConfiguredPreviewUsesConfiguredSettlementDefaults(t *testing.T) {
	catalog, err := rules.ConfiguredTemplateCatalog("canada_28")
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]string{}
	for _, play := range catalog.Plays {
		for _, outcome := range play.Decisions.Outcomes {
			if outcome.PriceKey != "" {
				prices[outcome.PriceKey] = "2"
			}
		}
	}
	engine, err := rules.NewConfiguredEngine(catalog, rules.ConfiguredBindings{Version: "explicit-test", Attributes: map[string]map[string][]int{}}, rules.ConfiguredPrices{Version: "explicit-test", Plates: map[string]map[string]string{"A": prices}})
	if err != nil {
		t.Fatal(err)
	}
	in := configuredPreviewInput{
		Snapshot: engine.Snapshot(),
		Requests: []rules.ConfiguredRequest{{PlayID: "big", Options: []string{"big"}, Plate: "A", Points: 10, Multiplier: 1}},
		Draw:     &rules.Draw{Digits: []int{9, 8, 6}},
	}
	out, err := configuredPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	result := out.(map[string]any)["result"].(rules.ConfiguredResult)
	if result.BetPoints != 10 || result.PrizePoints != 20 || result.RefundPoints != 0 {
		t.Fatalf("unexpected Canada preview result: %+v", result)
	}
}

func TestConfiguredPreviewRejectsIncompleteAndUnknownInputs(t *testing.T) {
	for _, mutate := range []func(*configuredPreviewInput){
		func(in *configuredPreviewInput) { in.Requests = nil },
		func(in *configuredPreviewInput) {
			in.Snapshot = json.RawMessage(strings.Replace(string(in.Snapshot), `"lucky_price":"2.5"`, `"other_price":"2.5"`, 1))
		},
		func(in *configuredPreviewInput) {
			in.Snapshot = json.RawMessage(strings.Replace(string(in.Snapshot), `"schema_version":2`, `"schema_version":2,"unknown":1`, 1))
		},
		func(in *configuredPreviewInput) {
			in.Snapshot = json.RawMessage(strings.Replace(string(in.Snapshot), `"schema_version":2`, `"schema_version":2,"schema_version":2`, 1))
		},
		func(in *configuredPreviewInput) { in.Requests[0].Options = []string{"missing"} },
		func(in *configuredPreviewInput) { in.Requests[0].Options = []string{"lucky", "lucky"} },
		func(in *configuredPreviewInput) { in.Draw = &rules.Draw{Special: []int{7}} },
	} {
		in := previewFixture(t)
		mutate(&in)
		if out, err := configuredPreview(context.Background(), in); err == nil {
			t.Fatalf("accepted invalid input: %v", out)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := configuredPreview(ctx, previewFixture(t)); err == nil {
		t.Fatal("accepted cancelled calculation")
	}
}

func TestConfiguredPreviewRoutesRequireManagementEntry(t *testing.T) {
	mux := buildRouter(Dependencies{})
	for _, prefix := range []string{"admin", "platform"} {
		for _, tc := range []struct{ method, path string }{{"GET", "/template"}, {"POST", ""}} {
			r := httptest.NewRequest(tc.method, "/api/v1/"+prefix+"/platform-games/rule-preview"+tc.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 503 {
				t.Fatalf("unconfigured auth returned %d: %s", w.Code, w.Body.String())
			}
		}
	}
	for _, route := range RegisteredRoutes() {
		if strings.Contains(route.Path, "rule-preview") && !strings.HasPrefix(route.Path, "/api/v1/admin/") && !strings.HasPrefix(route.Path, "/api/v1/platform/") {
			t.Fatal("public preview compute route", route)
		}
	}
}

func TestConfiguredCutoverErrorIsExplicit(t *testing.T) {
	out, err := betError(nil, betting.ErrLegacyPeriod)
	if err != nil || out.Status != 409 || out.Error == nil || out.Error.Code != "BET_LEGACY_PERIOD" {
		t.Fatal(out, err)
	}
}
