package application

import (
	"crypto/sha512"
	"encoding/base64"
	"testing"
)

func validLatest() Latest {
	digest := sha512.Sum512([]byte("package"))
	return Latest{
		Version: "1.2.3", Name: "nanokvm_1.2.3.tar.gz",
		Sha512: base64.StdEncoding.EncodeToString(digest[:]), LegacySize: 1,
	}
}

func TestValidateLatestV1DoesNotInterpretLegacySizeAsBytes(t *testing.T) {
	latest := validLatest()
	latest.LegacySize = 15048 // historic stable manifests used a non-byte value
	if err := validateLatest(&latest); err != nil {
		t.Fatal(err)
	}
	if err := validateDownloadedSize(&latest, 15406125); err != nil {
		t.Fatalf("v1 must not require equality with legacy size: %v", err)
	}
}

func TestValidateLatestV2RequiresExactByteFields(t *testing.T) {
	latest := validLatest()
	latest.ManifestVersion = 2
	latest.SizeBytes = 100
	latest.UnpackedSizeBytes = 200
	if err := validateLatest(&latest); err != nil {
		t.Fatal(err)
	}
	if err := validateDownloadedSize(&latest, 99); err == nil {
		t.Fatal("v2 size mismatch was accepted")
	}
	if err := validateExpandedSize(&latest, 199); err == nil {
		t.Fatal("v2 expanded size mismatch was accepted")
	}
	latest.ManifestVersion = 3
	if err := validateLatest(&latest); err == nil {
		t.Fatal("unknown manifest version was accepted")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"1.2.9", "1.3.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-beta.11", "1.0.0-rc.1", -1},
		{"1.0.0-rc.1", "1.0.0-rc.1", 0},
		{"1.0.0+build1", "1.0.0+build2", 0},
		{"1.0.1+build1", "1.0.0+build9", 1},
		{"1.0.0-rc.1+a", "1.0.0-rc.1+b", 0},
	}
	for _, tt := range tests {
		got, err := compareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareVersionsInvalid(t *testing.T) {
	for _, bad := range []string{"", "1", "1.2", "v1.2.3", "1.2.3-", "1.2.3-a..b", "a.b.c", "1.2.3 ", "99999999999999999999.0.0"} {
		if _, err := compareVersions(bad, "1.0.0"); err == nil {
			t.Errorf("compareVersions(%q, ...) accepted an invalid version", bad)
		}
		if _, err := compareVersions("1.0.0", bad); err == nil {
			t.Errorf("compareVersions(..., %q) accepted an invalid version", bad)
		}
	}
}

func TestCheckNotRollback(t *testing.T) {
	tests := []struct {
		name            string
		current, latest string
		allow           bool
		wantErr         bool
	}{
		{"newer", "1.2.3", "1.2.4", false, false},
		{"equal", "1.2.3", "1.2.3", false, true},
		{"older", "1.2.3", "1.2.2", false, true},
		{"older prerelease of same", "1.2.3", "1.2.3-rc.1", false, true},
		{"release over prerelease", "1.2.3-rc.1", "1.2.3", false, false},
		{"legacy default", "1.0.0", "1.1.0", false, false},
		{"older allowed", "1.2.3", "1.0.0", true, false},
		{"equal allowed", "1.2.3", "1.2.3", true, false},
		{"unparseable current refused", "garbage", "1.2.3", false, true},
		{"unparseable current allowed explicitly", "garbage", "1.2.3", true, false},
		{"unparseable latest refused", "1.2.3", "garbage", false, true},
	}
	for _, tt := range tests {
		err := checkNotRollback(tt.current, tt.latest, tt.allow)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: checkNotRollback(%q, %q, %v) = %v, wantErr %v", tt.name, tt.current, tt.latest, tt.allow, err, tt.wantErr)
		}
	}
}
