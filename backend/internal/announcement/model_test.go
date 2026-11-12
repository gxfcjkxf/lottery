package announcement

import (
	"strings"
	"testing"
	"time"
)

func TestAnnouncementHTMLRejectsExecutableAndForeignMarkup(t *testing.T) {
	for _, source := range []string{`<script>alert(1)</script>`, `<p onclick="x">hello</p>`, `<a href="javascript:alert(1)">x</a>`, `<img src="https://other/image.png">`, `<iframe src="/">x</iframe>`, `<svg onload="x"></svg>`, `<p style="position:fixed">x</p>`, `<!-- x --><p>hello</p>`} {
		if _, _, e := NormalizeHTML(source); e == nil {
			t.Fatalf("accepted %s", source)
		}
	}
	source := `<h2>Title</h2><p><strong>Bold</strong> <em>italic</em> <a href="/announcements">Link</a></p><img src="/api/v1/announcement-assets/` + strings.Repeat("a", 64) + `.png" alt="image">`
	normalized, assets, e := NormalizeHTML(source)
	if e != nil || len(assets) != 1 {
		t.Fatal(e, assets)
	}
	again, _, e := NormalizeHTML(normalized)
	if e != nil || again != normalized {
		t.Fatal("normalization not stable", e, normalized, again)
	}
}
func TestAnnouncementDayAndSchedule(t *testing.T) {
	now := time.Date(2026, 11, 11, 16, 1, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("Asia/Singapore")
	if Day(now, loc) != "2026-11-12" {
		t.Fatal("wrong daily boundary")
	}
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	v := Item{Status: "published", Config: Config{StartsAt: &future}}
	if State(v, now) != "scheduled" {
		t.Fatal(v)
	}
	v.Config = Config{EndsAt: &past}
	if State(v, now) != "ended" {
		t.Fatal(v)
	}
	v.Config = Config{}
	if State(v, now) != "active" {
		t.Fatal(v)
	}
}
