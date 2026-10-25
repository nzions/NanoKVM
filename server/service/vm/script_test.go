package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptPath(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()

	oldDir := ScriptDirectory
	ScriptDirectory = root
	t.Cleanup(func() { ScriptDirectory = oldDir })

	write := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(root, "ok.sh"))
	write(filepath.Join(root, "tool.py"))
	write(filepath.Join(outsideDir, "evil.sh"))
	if err := os.Symlink(filepath.Join(outsideDir, "evil.sh"), filepath.Join(root, "link.sh")); err != nil {
		t.Fatal(err)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   string
		want string // empty: must be refused
	}{
		{"existing script", "ok.sh", filepath.Join(realRoot, "ok.sh")},
		{"python script", "tool.py", filepath.Join(realRoot, "tool.py")},
		{"new script", "new.sh", filepath.Join(realRoot, "new.sh")},
		{"upper case extension", "NEW.SH", filepath.Join(realRoot, "NEW.SH")},
		{"empty", "", ""},
		{"traversal", "../evil.sh", ""},
		{"deep traversal", "../../../etc/passwd.sh", ""},
		{"absolute path", outsideDir + "/evil.sh", ""},
		{"subdirectory", "sub/x.sh", ""},
		{"leading dot", ".hidden.sh", ""},
		{"dot dot", "..sh", ""},
		{"shell metacharacters", "a;reboot.sh", ""},
		{"command substitution", "$(reboot).sh", ""},
		{"space", "a b.sh", ""},
		{"newline", "a\nb.sh", ""},
		{"backslash", `a\b.sh`, ""},
		{"wrong extension", "ok.txt", ""},
		{"no extension", "ok", ""},
		{"symlink leading out", "link.sh", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scriptPath(tt.in)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("scriptPath(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("scriptPath(%q) failed: %s", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("scriptPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
