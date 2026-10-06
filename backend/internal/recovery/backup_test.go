package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupVerificationRejectsDamageEvenWithSameSize(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.dump")
	if err := os.WriteFile(file, []byte("synthetic-content"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, n, err := FileDigest(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyBackup(file, hash, n); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("synthetic-damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = VerifyBackup(file, hash, n); err == nil {
		t.Fatal("same-size corrupt backup accepted")
	}
}

func TestBackupVerificationRejectsInvalidEvidenceOrMissingFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing.dump")
	for _, in := range []struct {
		hash string
		n    int64
	}{{"", 1}, {strings.Repeat("A", 64), 1}, {strings.Repeat("a", 64), 0}, {strings.Repeat("a", 64), 1}} {
		if err := VerifyBackup(file, in.hash, in.n); err == nil {
			t.Fatal("invalid or missing backup accepted")
		}
	}
}
