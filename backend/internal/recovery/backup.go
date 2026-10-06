package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func FileDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// VerifyBackup must succeed before restoration; an expected digest comes from
// separately retained evidence, not from recomputing a new manifest after damage.
func VerifyBackup(path, expectedSHA string, expectedBytes int64) error {
	if !lowerSHA256Pattern.MatchString(expectedSHA) || expectedBytes <= 0 {
		return fmt.Errorf("valid backup digest and positive size required")
	}
	hash, size, err := FileDigest(path)
	if err != nil {
		return err
	}
	if hash != expectedSHA || size != expectedBytes {
		return fmt.Errorf("backup digest or byte count does not match saved evidence")
	}
	return nil
}
