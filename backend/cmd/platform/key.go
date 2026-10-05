package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"os"
	"path/filepath"
	"strings"
)

func generateKey(path string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(base64.RawStdEncoding.EncodeToString(key))
	return err
}
func loadKey(c config.Config) ([]byte, error) {
	raw := c.AuthKey
	if c.AuthKeyFile != "" {
		data, err := os.ReadFile(c.AuthKeyFile)
		if err != nil {
			return nil, errors.New("AUTH_KEY_FILE cannot be read")
		}
		raw = strings.TrimSpace(string(data))
	}
	key, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("configure AUTH_KEY_FILE or AUTH_KEY with a base64 32-byte key")
	}
	return key, nil
}
