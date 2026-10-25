package download

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useImageDirectory(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	old := imageDirectory
	imageDirectory = dir
	t.Cleanup(func() { imageDirectory = old })

	return dir
}

func allowAllAddresses(t *testing.T) {
	t.Helper()

	old := ipAllowed
	ipAllowed = func(net.IP) bool { return true }
	t.Cleanup(func() { ipAllowed = old })
}

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"127.1.2.3", false},
		{"::1", false},
		{"10.0.0.1", false},
		{"172.16.5.4", false},
		{"192.168.1.1", false},
		{"100.64.0.1", false},
		{"100.127.255.254", false},
		{"169.254.169.254", false},
		{"fe80::1", false},
		{"fd00::1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"224.0.0.1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test address %q", tt.ip)
			}
			if got := isPublicIP(ip); got != tt.want {
				t.Fatalf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateImageFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"iso", "debian-12.iso", true},
		{"img", "disk.IMG", true},
		{"empty", "", false},
		{"traversal", "../x.iso", false},
		{"slash", "a/b.iso", false},
		{"leading dot", ".x.iso", false},
		{"space", "a b.iso", false},
		{"shell", "a;b.iso", false},
		{"wrong extension", "x.exe", false},
		{"no extension", "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImageFilename(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("validateImageFilename(%q) failed: %s", tt.in, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("validateImageFilename(%q) accepted a bad name", tt.in)
			}
		})
	}
}

func TestDownloadRefusesPrivateAddress(t *testing.T) {
	dir := useImageDirectory(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	sum := sha256.Sum256([]byte("payload"))

	// the httptest server listens on 127.0.0.1, which the default policy refuses
	err := (&Service{}).downloadRemoteImage(context.Background(), server.URL+"/x.iso", sum[:], "x.iso", nil)
	if !errors.Is(err, errBlockedAddress) {
		t.Fatalf("download from a loopback server: err = %v, want errBlockedAddress", err)
	}

	if _, err := os.Lstat(filepath.Join(dir, "x.iso")); err == nil {
		t.Fatal("image was stored")
	}
}

func TestDownloadRefusesRedirectToPrivateAddress(t *testing.T) {
	useImageDirectory(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/x.iso", http.StatusFound)
	}))
	defer redirector.Close()

	// let the first hop through and refuse the second
	old := ipAllowed
	t.Cleanup(func() { ipAllowed = old })
	calls := 0
	ipAllowed = func(net.IP) bool {
		calls++
		return calls == 1
	}

	sum := sha256.Sum256([]byte("payload"))
	err := (&Service{}).downloadRemoteImage(context.Background(), redirector.URL+"/x.iso", sum[:], "x.iso", nil)
	if !errors.Is(err, errBlockedAddress) {
		t.Fatalf("redirect to a private address: err = %v, want errBlockedAddress", err)
	}
}

func TestDownloadRemoteImage(t *testing.T) {
	allowAllAddresses(t)

	payload := []byte("an image")
	sum := sha256.Sum256(payload)
	wrong := sha256.Sum256([]byte("something else"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	tests := []struct {
		name     string
		sum      []byte
		filename string
		existing bool
		wantErr  error
		stored   bool
	}{
		{"matching checksum", sum[:], "ok.iso", false, nil, true},
		{"missing checksum", nil, "ok.iso", false, errSHA256Required, false},
		{"wrong checksum", wrong[:], "ok.iso", false, errSHA256Mismatch, false},
		{"existing file is not replaced", sum[:], "ok.iso", true, errImageExists, false},
		{"bad filename", sum[:], "../ok.iso", false, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := useImageDirectory(t)
			if tt.existing {
				if err := os.WriteFile(filepath.Join(dir, tt.filename), []byte("keep"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			err := (&Service{}).downloadRemoteImage(context.Background(), server.URL+"/x", tt.sum, tt.filename, nil)

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.stored:
				if err != nil {
					t.Fatalf("download failed: %s", err)
				}
			default:
				if err == nil {
					t.Fatal("download of a bad name succeeded")
				}
			}

			got, readErr := os.ReadFile(filepath.Join(dir, tt.filename))
			switch {
			case tt.stored:
				if readErr != nil || string(got) != string(payload) {
					t.Fatalf("stored image = %q, %v", got, readErr)
				}
			case tt.existing:
				if string(got) != "keep" {
					t.Fatalf("existing image was changed to %q", got)
				}
			}

			// no temporary file stays behind
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".nanokvm-download-") {
					t.Fatalf("temporary file %s left behind", entry.Name())
				}
			}
		})
	}
}
