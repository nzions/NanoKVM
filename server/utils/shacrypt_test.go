package utils

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Test vectors from Drepper's "Unix crypt using SHA-256 and SHA-512".
func TestSHA512Crypt(t *testing.T) {
	tests := []struct {
		name     string
		password string
		salt     string
		rounds   int
		want     string
	}{
		{"default rounds", "Hello world!", "saltstring", 0,
			"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"rounds 10000, salt cut to 16", "Hello world!", "saltstringsaltstring", 10000,
			"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v."},
		{"explicit 5000", "This is just a test", "toolongsaltstring", 5000,
			"$6$rounds=5000$toolongsaltstrin$lQ8jolhgVRVhY4b5pZKaysCLi0QBxGoNeKQzQ3glMhwllF7oGDZxUhx1yxdYcz/e1JSbq3y6JMxxl8audkUEm0"},
		{"rounds 1400", "a very much longer text to encrypt.  This one even stretches over morethan one line.", "anotherlongsaltstring", 1400,
			"$6$rounds=1400$anotherlongsalts$POfYwTEok97VWcjxIiSOjiykti.o/pQs.wPvMxQ6Fm7I6IoYN3CmLs66x9t0oSwbtEW7o7UmJEiDwGqd8p4ur1"},
		{"rounds 77777", "we have a short salt string but not a short password", "short", 77777,
			"$6$rounds=77777$short$WuQyW2YR.hBNpjjRhpYD/ifIw05xdfeEyQoMxIXbkvr0gge1a1x3yRULJ5CCaUeOxFmtlcGZelFl5CxtgfiAc0"},
		{"rounds 123456", "a short string", "asaltof16chars..", 123456,
			"$6$rounds=123456$asaltof16chars..$BtCwjqMJGx5hrJhZywWvt0RLE8uZ4oPwcelCjmw2kSYu.Ec6ycULevoBK25fs2xXgMNrCzIMVcgEJAstJeonj1"},
		{"rounds clamped up", "the minimum number is still observed", "roundstoolow", 10,
			"$6$rounds=1000$roundstoolow$kUMsbe306n21p9R.FRkW3IGn.S9NPN0x50YhH1xhLsPuWGsUSklZt58jaTfF4ZEQpyUNGc0dqbpBYYBaHHrsX."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SHA512Crypt(tt.password, tt.salt, tt.rounds); got != tt.want {
				t.Errorf("SHA512Crypt(%q, %q, %d)\n got %s\nwant %s", tt.password, tt.salt, tt.rounds, got, tt.want)
			}
		})
	}
}

func TestSetRootPassword(t *testing.T) {
	dir := t.TempDir()
	orig := ShadowFile
	ShadowFile = filepath.Join(dir, "shadow")
	t.Cleanup(func() { ShadowFile = orig })

	before := "daemon:*:19000:0:99999:7:::\nroot:$5$abc$def:19000:0:99999:7:::\nnik:$6$x$y:19001:0:99999:7:::\n"
	if err := os.WriteFile(ShadowFile, []byte(before), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ShadowFile, 0o640); err != nil {
		t.Fatal(err)
	}

	if err := SetRootPassword("s3cret-pass"); err != nil {
		t.Fatalf("SetRootPassword: %v", err)
	}

	data, err := os.ReadFile(ShadowFile)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Split(before, "\n")
	got := strings.Split(string(data), "\n")
	if len(got) != len(old) {
		t.Fatalf("line count changed: %d -> %d", len(old), len(got))
	}
	for i := range old {
		if i != 1 {
			if got[i] != old[i] {
				t.Errorf("line %d changed: %q -> %q", i, old[i], got[i])
			}
			continue
		}
		of := strings.Split(old[i], ":")
		nf := strings.Split(got[i], ":")
		if len(nf) != len(of) {
			t.Fatalf("root field count changed: %q", got[i])
		}
		for j := range of {
			changed := of[j] != nf[j]
			if (j == 1 || j == 2) != changed {
				t.Errorf("root field %d: %q -> %q", j, of[j], nf[j])
			}
		}
		if !strings.HasPrefix(nf[1], "$6$") || len(nf[1]) != 3+16+1+86 {
			t.Errorf("unexpected hash %q", nf[1])
		}
		salt := strings.Split(nf[1], "$")[2]
		if want := SHA512Crypt("s3cret-pass", salt, 0); nf[1] != want {
			t.Errorf("hash does not verify: %q != %q", nf[1], want)
		}
		if _, err := strconv.Atoi(nf[2]); err != nil || nf[2] == "19000" {
			t.Errorf("lastchg not updated: %q", nf[2])
		}
	}

	info, err := os.Stat(ShadowFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("leftover files in %s: %d entries", dir, len(entries))
	}
}

func TestSetRootPasswordNoRootLine(t *testing.T) {
	dir := t.TempDir()
	orig := ShadowFile
	ShadowFile = filepath.Join(dir, "shadow")
	t.Cleanup(func() { ShadowFile = orig })

	before := "daemon:*:19000:0:99999:7:::\nnik:$6$x$y:19001:0:99999:7:::\n"
	if err := os.WriteFile(ShadowFile, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetRootPassword("s3cret-pass"); err == nil {
		t.Fatal("expected an error when there is no root line")
	}
	data, err := os.ReadFile(ShadowFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Errorf("shadow modified: %q", data)
	}
}

func TestSetRootPasswordInvalid(t *testing.T) {
	for _, pw := range []string{"", "a:b", "a\nb", "a\rb"} {
		if err := SetRootPassword(pw); err == nil {
			t.Errorf("SetRootPassword(%q) did not fail", pw)
		}
	}
}
