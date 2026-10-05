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
	if _, err := normalizePhone("+639171234567"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"09171234567", "+0000123", "+123x"} {
		if _, err := normalizePhone(value); err == nil {
			t.Fatal(value)
		}
	}
}
