package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Generate random string for secret key. There is no fallback: a secret that
// cannot be drawn from crypto/rand must stop the server, not weaken it.
func generateRandomSecretKey() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

// loadOrCreateJWTSecret returns the secret stored at path (etcDir/.jwt_secret
// when server.yaml carries none, so a restart does not log everyone out),
// creating it 0600 when missing.
func loadOrCreateJWTSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if secret := strings.TrimSpace(string(data)); secret != "" {
			return secret, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	secret, err := generateRandomSecretKey()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err = os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	// the file may already exist with a looser mode
	if err = os.Chmod(path, 0o600); err != nil {
		return "", err
	}

	return secret, nil
}
