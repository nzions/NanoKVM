package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateJWTSecretPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ".jwt_secret")

	first, err := loadOrCreateJWTSecret(path)
	if err != nil || first == "" {
		t.Fatalf("create: secret=%q err=%v", first, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v, want 0600", info.Mode().Perm())
	}

	second, err := loadOrCreateJWTSecret(path)
	if err != nil || second != first {
		t.Fatalf("reload: secret=%q want %q err=%v", second, first, err)
	}
}

func TestLoadOrCreateJWTSecretFailsClosed(t *testing.T) {
	// a directory where the file should be: reading and writing both fail
	path := t.TempDir()
	if secret, err := loadOrCreateJWTSecret(path); err == nil {
		t.Fatalf("expected an error, got secret %q", secret)
	}
}

func TestGenerateRandomSecretKey(t *testing.T) {
	a, err := generateRandomSecretKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateRandomSecretKey()
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b {
		t.Fatalf("secrets must be non-empty and distinct: %q %q", a, b)
	}
}
