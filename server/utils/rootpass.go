package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RootPassFile holds a root password the device generated on its own (written
// 0600 by the init scripts when the image's default root:root is found at boot).
// kvm_system shows it on the OLED while it exists; it is removed once the
// owner has chosen a password.
const RootPassFile = "/etc/kvm/root.pass"

// SetRootPassword sets root's Linux password with chpasswd. The password goes
// over stdin, never argv, and no shell is involved.
func SetRootPassword(password string) error {
	if password == "" || strings.ContainsAny(password, "\r\n:") {
		return fmt.Errorf("refusing to set an invalid root password")
	}

	var output bytes.Buffer
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader("root:" + password + "\n")
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chpasswd failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

// ClearRootPassFile removes the device-generated root password file; a missing
// file is not an error.
func ClearRootPassFile() error {
	if err := os.Remove(RootPassFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
