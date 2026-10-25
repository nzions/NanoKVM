package download

import (
	"context"
	"crypto/sha256"
	"errors"
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

func TestDownloadRemoteImage(t *testing.T) {

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
		{"missing checksum is stored unverified", nil, "ok.iso", false, nil, true},
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
