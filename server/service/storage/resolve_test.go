package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveImagePath(t *testing.T) {
	root := t.TempDir()
	// a sibling whose name starts with the image directory name
	sibling := root + "-other"
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })

	oldDir := imageDirectory
	imageDirectory = root
	t.Cleanup(func() { imageDirectory = oldDir })

	write := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	good := filepath.Join(root, "good.iso")
	upper := filepath.Join(root, "UPPER.IMG")
	text := filepath.Join(root, "notes.txt")
	outside := filepath.Join(sibling, "evil.img")
	secret := filepath.Join(sibling, "secret.txt")
	write(good)
	write(upper)
	write(text)
	write(outside)
	write(secret)

	// links inside the directory: one to an image outside, one to a non-image
	// outside, one to an image inside
	escapeLink := filepath.Join(root, "escape.img")
	if err := os.Symlink(outside, escapeLink); err != nil {
		t.Fatal(err)
	}
	extLink := filepath.Join(root, "disguised.iso")
	if err := os.Symlink(secret, extLink); err != nil {
		t.Fatal(err)
	}
	innerLink := filepath.Join(root, "inner.iso")
	if err := os.Symlink(good, innerLink); err != nil {
		t.Fatal(err)
	}
	dirLink := filepath.Join(root, "dir")
	if err := os.Symlink(sibling, dirLink); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(root, "sub.iso")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	realGood, err := filepath.EvalSymlinks(good)
	if err != nil {
		t.Fatal(err)
	}
	realUpper, err := filepath.EvalSymlinks(upper)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		file string
		want string // empty: must be refused
	}{
		{"image inside", good, realGood},
		{"upper case extension", upper, realUpper},
		{"unclean path inside", root + "/./good.iso", realGood},
		{"link to image inside", innerLink, realGood},
		{"empty", "", ""},
		{"relative", "good.iso", ""},
		{"dot dot out of the directory", root + "/../" + filepath.Base(sibling) + "/evil.img", ""},
		{"dot dot inside the name", root + "/sub/../../" + filepath.Base(sibling) + "/evil.img", ""},
		{"sibling with the same prefix", outside, ""},
		{"symlink to an image outside", escapeLink, ""},
		{"symlink to a file outside", extLink, ""},
		{"through a directory symlink", filepath.Join(dirLink, "evil.img"), ""},
		{"wrong extension", text, ""},
		{"directory named like an image", subdir, ""},
		{"missing file", filepath.Join(root, "missing.iso"), ""},
		{"the directory itself", root, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveImagePath(tt.file)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("resolveImagePath(%q) = %q, want an error", tt.file, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveImagePath(%q) failed: %s", tt.file, err)
			}
			if got != tt.want {
				t.Fatalf("resolveImagePath(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}
