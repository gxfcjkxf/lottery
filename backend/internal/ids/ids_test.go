package ids

import (
	"regexp"
	"testing"
)

func TestUUIDv7(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		id := New()
		if !pattern.MatchString(id) || seen[id] {
			t.Fatalf("invalid or duplicate: %s", id)
		}
		seen[id] = true
	}
}
