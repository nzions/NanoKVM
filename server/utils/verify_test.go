package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Vectors from Drepper's specification, and openssl passwd -1 for MD5-crypt.
func TestSHA256Crypt(t *testing.T) {
	tests := []struct {
		name     string
		password string
		salt     string
		rounds   int
		want     string
	}{
		{"default rounds", "Hello world!", "saltstring", 0,
			"$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5"},
		{"rounds 10000, salt cut to 16", "Hello world!", "saltstringsaltstring", 10000,
			"$5$rounds=10000$saltstringsaltst$3xv.VbSHBb41AL9AvLeujZkZRBAwqFMz2.opqey6IcA"},
		{"rounds 77777", "we have a short salt string but not a short password", "short", 77777,
			"$5$rounds=77777$short$JiO1O3ZpDAxGJeaDIuqCoEFysAe1mZNJRs3pw0KQRd/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SHA256Crypt(tt.password, tt.salt, tt.rounds); got != tt.want {
				t.Errorf("SHA256Crypt\n got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestMD5Crypt(t *testing.T) {
	tests := []struct{ password, salt, want string }{
		{"password", "xxxxxxxx", "$1$xxxxxxxx$UYCIxa628.9qXjpQCjM4a."},
		{"", "ab", "$1$ab$rn6aQS/o7141mj179E/zA."},
		{"a long password over sixteen bytes", "saltsalt", "$1$saltsalt$s.AGSOKXM9SWDAgknA9Iu1"},
	}
	for _, tt := range tests {
		if got := MD5Crypt(tt.password, tt.salt); got != tt.want {
			t.Errorf("MD5Crypt(%q, %q) = %s, want %s", tt.password, tt.salt, got, tt.want)
		}
	}
}

func TestVerifyCrypt(t *testing.T) {
	good := []struct{ hash, password string }{
		{"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1", "Hello world!"},
		{"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v.", "Hello world!"},
		{"$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5", "Hello world!"},
		{"$5$rounds=10000$saltstringsaltst$3xv.VbSHBb41AL9AvLeujZkZRBAwqFMz2.opqey6IcA", "Hello world!"},
		{"$1$xxxxxxxx$UYCIxa628.9qXjpQCjM4a.", "password"},
	}
	for _, tt := range good {
		ok, err := VerifyCrypt(tt.hash, tt.password)
		if err != nil || !ok {
			t.Errorf("VerifyCrypt(%.20s...) = %v, %v; want true", tt.hash, ok, err)
		}
		ok, err = VerifyCrypt(tt.hash, tt.password+"x")
		if err != nil || ok {
			t.Errorf("VerifyCrypt(%.20s...) wrong password = %v, %v; want false, nil", tt.hash, ok, err)
		}
	}
}

func TestVerifyCryptUnverifiable(t *testing.T) {
	for _, hash := range []string{
		"", "!", "*", "!$6$a$b", "*LOCKED*", "xyz12345abcde", "$y$j9T$salt$hash", "$2b$10$abc",
		"$6$", "$6$nosaltend", "$6$rounds=abc$salt$hash", "$6$rounds=5000", "$5$salt$", "$1$nodollar", "$1$toolongsalt123$abc",
	} {
		ok, err := VerifyCrypt(hash, "anything")
		if ok || !errors.Is(err, ErrUnverifiableHash) {
			t.Errorf("VerifyCrypt(%q) = %v, %v; want false and ErrUnverifiableHash", hash, ok, err)
		}
	}
}

func TestVerifyRootPassword(t *testing.T) {
	orig := ShadowFile
	ShadowFile = filepath.Join(t.TempDir(), "shadow")
	t.Cleanup(func() { ShadowFile = orig })

	hash := SHA512Crypt("correct horse", "abcdefgh", 0)
	shadow := "daemon:*:19000:0:99999:7:::\nroot:" + hash + ":19000:0:99999:7:::\nnik:$6$x$y:19001:::::\n"
	if err := os.WriteFile(ShadowFile, []byte(shadow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckRootHashVerifiable(); err != nil {
		t.Fatalf("CheckRootHashVerifiable: %v", err)
	}
	if ok, err := VerifyRootPassword("correct horse"); err != nil || !ok {
		t.Errorf("right password = %v, %v", ok, err)
	}
	if ok, err := VerifyRootPassword("wrong"); err != nil || ok {
		t.Errorf("wrong password = %v, %v", ok, err)
	}

	for name, body := range map[string]string{
		"locked":  "root:!:19000:0:99999:7:::\n",
		"empty":   "root::19000:0:99999:7:::\n",
		"des":     "root:abJnggxhB/yWI:19000:0:99999:7:::\n",
		"no root": "daemon:*:19000:0:99999:7:::\n",
		"short":   "root:$6$a$b\n",
	} {
		if err := os.WriteFile(ShadowFile, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckRootHashVerifiable(); err == nil {
			t.Errorf("%s: CheckRootHashVerifiable = nil, want an error", name)
		}
		if ok, _ := VerifyRootPassword("x"); ok {
			t.Errorf("%s: VerifyRootPassword matched", name)
		}
	}
}
