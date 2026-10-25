package auth

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the config package at a scratch directory so
// config.GetInstance() never touches /etc/kvm.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nanokvm-etc-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("NANOKVM_ETC_DIR", dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
