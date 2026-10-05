package mutation

import "testing"

func TestSealedResponseAndFingerprint(t *testing.T) {
	e, err := New(nil, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"access_token":"private-token"}`)
	encrypted, err := e.seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if string(encrypted) == string(plaintext) {
		t.Fatal("response not encrypted")
	}
	got, err := e.open(encrypted)
	if err != nil || string(got) != string(plaintext) {
		t.Fatal("decrypt", err)
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err = e.open(encrypted); err == nil {
		t.Fatal("tamper accepted")
	}
	if e.Fingerprint("password1") == e.Fingerprint("password2") {
		t.Fatal("fingerprints collide")
	}
	other, _ := New(nil, append([]byte{1}, make([]byte, 31)...))
	if e.Fingerprint("password1") == other.Fingerprint("password1") {
		t.Fatal("fingerprint is not keyed")
	}
	bound, _ := e.sealBound(plaintext, []byte("scope-A"))
	if _, err = e.openBound(bound, []byte("scope-B")); err == nil {
		t.Fatal("encrypted result moved across operation scopes")
	}
}
func TestKeyAndIdempotencyValidation(t *testing.T) {
	if _, err := New(nil, []byte("short")); err == nil {
		t.Fatal("short encryption key")
	}
	for _, key := range []string{"", "abc", "a bbbbbbb"} {
		if validKey(key) {
			t.Fatal("invalid key accepted", key)
		}
	}
	if !validKey("operation-00001") {
		t.Fatal("valid key rejected")
	}
}
