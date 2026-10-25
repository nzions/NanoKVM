package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHostname(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	label64 := strings.Repeat("a", 64)
	// four 63 character labels and dots are 255 characters
	long := strings.Join([]string{label63, label63, label63, label63}, ".")

	tests := []struct {
		name     string
		hostname string
		ok       bool
	}{
		{"simple", "nanokvm", true},
		{"digits and hyphen", "kvm-01", true},
		{"fqdn", "kvm.example.com", true},
		{"single character", "a", true},
		{"longest label", label63, true},
		{"longest name", strings.Join([]string{label63, label63, label63, strings.Repeat("a", 61)}, "."), true},
		{"empty", "", false},
		{"label too long", label64, false},
		{"name too long", long, false},
		{"leading hyphen", "-kvm", false},
		{"trailing hyphen", "kvm-", false},
		{"hyphen after a dot", "a.-b", false},
		{"hyphen before a dot", "a-.b", false},
		{"empty label", "a..b", false},
		{"leading dot", ".kvm", false},
		{"trailing dot", "kvm.", false},
		{"upper case", "KVM", false},
		{"underscore", "kvm_1", false},
		{"space", "kvm 1", false},
		{"newline", "kvm\n127.0.0.1 evil", false},
		{"slash", "../etc", false},
		{"shell", "$(reboot)", false},
		{"non ascii", "kvmé", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHostname(tt.hostname)
			if tt.ok && err != nil {
				t.Fatalf("validateHostname(%q) failed: %s", tt.hostname, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("validateHostname(%q) accepted a bad hostname", tt.hostname)
			}
		})
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostname")

	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("read back %q, %v", got, err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v, %v", info.Mode(), err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v, %v", entries, err)
	}

	// a missing directory fails and leaves nothing behind
	if err := writeFileAtomic(filepath.Join(dir, "missing", "hostname"), []byte("x"), 0o644); err == nil {
		t.Fatal("write into a missing directory succeeded")
	}
}
