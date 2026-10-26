package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RootPassFile holds a root password the device generated on its own (written
// 0600 by the init scripts when the image's default root:root is found at boot).
// kvm_system shows it on the OLED while it exists; it is removed once the
// owner has chosen a password. A var so a test can redirect it.
var RootPassFile = "/etc/kvm/root.pass"

// ShadowFile is the shadow database SetRootPassword rewrites; a var so a test
// can redirect it.
var ShadowFile = "/etc/shadow"

// SetRootPassword sets root's Linux password by rewriting root's entry in
// ShadowFile with a SHA-512 crypt hash. The stock image has no chpasswd (not
// even as a busybox applet), so nothing is exec'd and the password never
// reaches argv. Only root's hash and last-change day change; every other line
// is kept byte for byte, and a missing root line is an error (never appended).
func SetRootPassword(password string) error {
	if password == "" || strings.ContainsAny(password, "\r\n:\x00") {
		return fmt.Errorf("refusing to set an invalid root password")
	}

	salt, err := randomSalt(shaCryptMaxSalt)
	if err != nil {
		return fmt.Errorf("make a password salt: %w", err)
	}
	hash := SHA512Crypt(password, salt, 0)

	return rewriteShadowRoot(ShadowFile, hash, time.Now().UTC().Unix()/86400)
}

// randomSalt returns n characters of the crypt alphabet from crypto/rand.
func randomSalt(n int) (string, error) {
	limit := big.NewInt(int64(len(shaCryptAlphabet)))
	out := make([]byte, n)
	for i := range out {
		v, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = shaCryptAlphabet[v.Int64()]
	}
	return string(out), nil
}

// rewriteShadowRoot atomically replaces fields 2 and 3 of the root line in the
// shadow file at path.
func rewriteShadowRoot(path, hash string, days int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("stat %s: no uid/gid available", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		fields := strings.Split(line, ":")
		if fields[0] != "root" {
			continue
		}
		if len(fields) < 3 {
			return fmt.Errorf("root entry in %s is malformed", path)
		}
		fields[1] = hash
		fields[2] = strconv.FormatInt(days, 10)
		lines[i] = strings.Join(fields, ":")
		found = true
		break
	}
	if !found {
		return fmt.Errorf("no root entry in %s", path)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".new")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	done := false
	cleanup := func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}
	defer cleanup()

	if _, err := tmp.WriteString(strings.Join(lines, "\n")); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Chown(int(st.Uid), int(st.Gid)); err != nil {
		return fmt.Errorf("chown %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s: %w", tmpName, err)
	}
	done = true

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	return d.Close()
}

// ClearRootPassFile removes the device-generated root password file; a missing
// file is not an error.
func ClearRootPassFile() error {
	if err := os.Remove(RootPassFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
