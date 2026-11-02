package identity

import "testing"

func TestNormalizeIdentifiers(t *testing.T) {
	if got, err := normalizeUsername(" Alice_1 "); err != nil || got != "alice_1" {
		t.Fatal(got, err)
	}
	for _, value := range []string{"a", "abc@example.com", "a b", "汉字名字"} {
		if _, err := normalizeUsername(value); err == nil {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"+639171234567", "09171234567", "13455676549", "81234567", "+8613455676549"} {
		if got, err := normalizePhone(" " + value + " "); err != nil || got != value {
			t.Fatal(value, got, err)
		}
	}
	for _, value := range []string{"123456", "1234567890123456", "+0000123", "+123x", "123+4567890"} {
		if _, err := normalizePhone(value); err == nil {
			t.Fatal(value)
		}
	}
}
