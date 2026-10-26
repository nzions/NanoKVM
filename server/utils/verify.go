package utils

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ErrUnverifiableHash is returned for a stored password hash this package
// cannot check: a locked or empty field, DES, yescrypt, or a malformed one.
var ErrUnverifiableHash = errors.New("password hash is not verifiable")

// VerifyCrypt reports whether password matches the crypt(3) hash. It checks
// $6$ (SHA-512), $5$ (SHA-256) and $1$ (MD5), the formats the stock image and
// busybox passwd write; any other form is ErrUnverifiableHash, never a match.
func VerifyCrypt(hash, password string) (bool, error) {
	switch {
	case strings.HasPrefix(hash, "$6$"), strings.HasPrefix(hash, "$5$"):
		magic := hash[:3]
		rest := hash[3:]
		rounds := 0
		if strings.HasPrefix(rest, "rounds=") {
			end := strings.IndexByte(rest, '$')
			if end < 0 {
				return false, fmt.Errorf("%w: malformed rounds", ErrUnverifiableHash)
			}
			n, err := strconv.Atoi(rest[len("rounds="):end])
			if err != nil || n <= 0 {
				return false, fmt.Errorf("%w: malformed rounds", ErrUnverifiableHash)
			}
			rounds = n
			rest = rest[end+1:]
		}
		end := strings.IndexByte(rest, '$')
		if end < 0 || end > shaCryptMaxSalt || end == len(rest)-1 {
			return false, fmt.Errorf("%w: malformed %s hash", ErrUnverifiableHash, magic)
		}
		salt := rest[:end]
		var got string
		if magic == "$6$" {
			got = SHA512Crypt(password, salt, rounds)
		} else {
			got = SHA256Crypt(password, salt, rounds)
		}
		return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1, nil
	case strings.HasPrefix(hash, "$1$"):
		rest := hash[3:]
		end := strings.IndexByte(rest, '$')
		if end < 0 || end > md5CryptMaxSalt || end == len(rest)-1 {
			return false, fmt.Errorf("%w: malformed $1$ hash", ErrUnverifiableHash)
		}
		got := MD5Crypt(password, rest[:end])
		return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1, nil
	case hash == "":
		return false, fmt.Errorf("%w: empty", ErrUnverifiableHash)
	case hash[0] == '!' || hash[0] == '*':
		return false, fmt.Errorf("%w: locked", ErrUnverifiableHash)
	default:
		return false, fmt.Errorf("%w: unsupported type", ErrUnverifiableHash)
	}
}

// RootHash returns the password hash field of root's entry in ShadowFile.
func RootHash() (string, error) {
	f, err := os.Open(ShadowFile)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", ShadowFile, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if fields[0] != "root" {
			continue
		}
		if len(fields) < 3 {
			return "", fmt.Errorf("root entry in %s is malformed", ShadowFile)
		}
		return fields[1], nil
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", ShadowFile, err)
	}
	return "", fmt.Errorf("no root entry in %s", ShadowFile)
}

// VerifyRootPassword reports whether password is root's Linux password.
func VerifyRootPassword(password string) (bool, error) {
	hash, err := RootHash()
	if err != nil {
		return false, err
	}
	return VerifyCrypt(hash, password)
}

// CheckRootHashVerifiable returns nil only when root's stored hash is of a
// type VerifyCrypt can check, so web login can rely on it. It does not know
// the password.
func CheckRootHashVerifiable() error {
	hash, err := RootHash()
	if err != nil {
		return err
	}
	// a probe password: only the format matters, the match result is ignored
	if _, err = VerifyCrypt(hash, "x"); err != nil {
		return err
	}
	return nil
}
