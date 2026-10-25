package application

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readTree returns every regular file under root (relative path -> content) and
// fails the test if any file mode is not 0755 when wantMode is set.
func readTree(t *testing.T, root string, wantMode os.FileMode) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if wantMode != 0 && info.Mode().Perm() != wantMode {
			t.Errorf("%s has mode %v, want %v", p, info.Mode().Perm(), wantMode)
		}
		if info.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		got[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertTree(t *testing.T, root string, want map[string]string, wantMode os.FileMode) {
	t.Helper()
	got := readTree(t, root, wantMode)
	if len(got) != len(want) {
		t.Errorf("%s: got files %v, want %v", root, got, want)
	}
	for name, content := range want {
		if got[name] != content {
			t.Errorf("%s/%s = %q, want %q", root, name, got[name], content)
		}
	}
}

var (
	oldFiles = map[string]string{
		"version":               "1.0.0",
		"server/NanoKVM-Server": "old-server",
		"server/web/old.html":   "old-web",
	}
	newFiles = map[string]string{
		"version":               "2.0.0",
		"server/NanoKVM-Server": "new-server",
		"system/init.d/S95":     "new-init",
	}
)

func TestInstallTree(t *testing.T) {
	base := t.TempDir()
	appDir := filepath.Join(base, "kvmapp")
	sourceDir := filepath.Join(base, "work", "src")
	stagingDir := filepath.Join(base, "kvmapp.new")
	backupDir := filepath.Join(base, "old")
	writeTree(t, appDir, oldFiles)
	writeTree(t, sourceDir, newFiles)

	if err := installTree(sourceDir, appDir, stagingDir, backupDir); err != nil {
		t.Fatalf("installTree: %v", err)
	}

	assertTree(t, appDir, newFiles, 0o755)
	assertTree(t, backupDir, oldFiles, 0)
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Errorf("staging dir still exists: %v", err)
	}
}

func TestInstallTreeRemovesStaleStaging(t *testing.T) {
	base := t.TempDir()
	appDir := filepath.Join(base, "kvmapp")
	sourceDir := filepath.Join(base, "work", "src")
	stagingDir := filepath.Join(base, "kvmapp.new")
	backupDir := filepath.Join(base, "old")
	writeTree(t, appDir, oldFiles)
	writeTree(t, sourceDir, newFiles)
	writeTree(t, stagingDir, map[string]string{"stale": "leftover"})
	writeTree(t, backupDir, map[string]string{"older": "previous backup"})

	if err := installTree(sourceDir, appDir, stagingDir, backupDir); err != nil {
		t.Fatalf("installTree: %v", err)
	}

	assertTree(t, appDir, newFiles, 0o755)
	assertTree(t, backupDir, oldFiles, 0)
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Errorf("staging dir still exists: %v", err)
	}
}

func TestInstallTreeMissingSourceLeavesAppUntouched(t *testing.T) {
	base := t.TempDir()
	appDir := filepath.Join(base, "kvmapp")
	stagingDir := filepath.Join(base, "kvmapp.new")
	backupDir := filepath.Join(base, "old")
	writeTree(t, appDir, oldFiles)

	if err := installTree(filepath.Join(base, "nope"), appDir, stagingDir, backupDir); err == nil {
		t.Fatal("installTree succeeded with a missing source dir")
	}

	assertTree(t, appDir, oldFiles, 0)
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Errorf("backup dir exists after a failed install: %v", err)
	}
}
