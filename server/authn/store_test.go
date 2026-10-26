package authn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const testSeedPassword = "seed-password"

// seededStore returns a store whose first-boot password is testSeedPassword.
func seededStore(t *testing.T, path string) *Store {
	t.Helper()
	store := NewStore(path)
	if err := os.WriteFile(store.seedPath, []byte(testSeedPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestFreshSeedIsRandomPrivateAndMustChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	store := NewStore(path)
	if _, ok, err := store.Authenticate("admin", "admin"); err != nil || ok {
		t.Fatalf("known default must not work: ok=%v err=%v", ok, err)
	}
	data, err := os.ReadFile(store.seedPath)
	if err != nil {
		t.Fatal(err)
	}
	password := strings.TrimSpace(string(data))
	if len(password) < seedPasswordLength {
		t.Fatalf("seed password too short: %q", password)
	}
	info, err := os.Stat(store.seedPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("seed file mode: %v, %v", info, err)
	}
	user, ok, err := store.Authenticate("admin", password)
	if err != nil || !ok || !user.MustChangePassword {
		t.Fatalf("seed login: user=%+v ok=%v err=%v", user, ok, err)
	}
	if _, err = store.SetPassword("admin", "a-new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(store.seedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("seed file must be removed after the owner changes the password: %v", err)
	}
}

func TestInitReportsTheSeededPasswordOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	store := NewStore(path)

	seeded, err := store.Init()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if seeded == "" || seeded != strings.TrimSpace(string(data)) {
		t.Fatalf("Init reported %q, seed file holds %q", seeded, data)
	}

	// the account exists now: nothing new is seeded, so root is left alone
	seeded, err = store.Init()
	if err != nil || seeded != "" {
		t.Fatalf("second Init: seeded=%q err=%v, want empty", seeded, err)
	}
}

func TestLegacyDefaultPasswordMustChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	hash, err := bcrypt.GenerateFromPassword([]byte(legacyDefaultPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(legacyAccount{Username: "admin", Password: string(hash)})
	if err = os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	user, ok, err := NewStore(path).Authenticate("admin", legacyDefaultPassword)
	if err != nil || !ok || !user.MustChangePassword {
		t.Fatalf("legacy default must be forced to change: user=%+v ok=%v err=%v", user, ok, err)
	}
}

func TestLegacyAccountMigratesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(legacyAccount{Username: "owner", Password: string(hash)})
	if err = os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(path)
	user, ok, err := store.Authenticate("owner", "legacy-password")
	if err != nil || !ok {
		t.Fatalf("authenticate migrated account: ok=%v err=%v", ok, err)
	}
	if user.Role != RoleAdmin || !user.SystemAccount || user.TokenVersion == 0 {
		t.Fatalf("unexpected migrated user: %+v", user)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var db database
	if err = json.Unmarshal(data, &db); err != nil {
		t.Fatalf("account file was not migrated: %v", err)
	}
	if db.Version != currentFileVersion || len(db.Users) != 1 {
		t.Fatalf("unexpected database: %+v", db)
	}
	var downgrade legacyAccount
	if err = json.Unmarshal(data, &downgrade); err != nil || downgrade.Username != "owner" || downgrade.Password == "" {
		t.Fatalf("legacy downgrade mirror is invalid: %+v, %v", downgrade, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("account mode = %o, want 600", info.Mode().Perm())
	}
}

func TestCorruptAccountFileFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, ok, err := NewStore(path).Authenticate("admin", testSeedPassword)
	if err == nil || ok {
		t.Fatalf("corrupt file must fail closed: ok=%v err=%v", ok, err)
	}
}

func TestUserLifecycleAndTokenRevocation(t *testing.T) {
	store := seededStore(t, filepath.Join(t.TempDir(), "pwd"))
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err := store.Create("alice", "correct-horse", RoleUser); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Authenticate("alice", "wrong-password"); err != nil || ok {
		t.Fatalf("wrong password login: ok=%v err=%v", ok, err)
	}
	alice, err := store.Get("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ValidateToken("alice", alice.TokenVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetPassword("alice", "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ValidateToken("alice", alice.TokenVersion); err == nil {
		t.Fatal("old token stayed valid after password change")
	}
	if _, ok, err := store.Authenticate("alice", "new-password"); err != nil || !ok {
		t.Fatalf("new password login: ok=%v err=%v", ok, err)
	}

	disabled := false
	if _, err = store.Update("admin", "alice", UserPatch{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Authenticate("alice", "new-password"); err != nil || ok {
		t.Fatalf("disabled login: ok=%v err=%v", ok, err)
	}
	enabled := true
	if _, err = store.Update("admin", "alice", UserPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete("admin", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get("alice"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deleted user lookup error = %v", err)
	}
}

func TestLastAdminAndSelfProtection(t *testing.T) {
	store := seededStore(t, filepath.Join(t.TempDir(), "pwd"))
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	userRole := RoleUser
	if _, err := store.Update("admin", "admin", UserPatch{Role: &userRole}); !errors.Is(err, ErrSelfModification) {
		t.Fatalf("self demotion error = %v", err)
	}
	if err := store.Delete("admin", "admin"); !errors.Is(err, ErrSelfDelete) {
		t.Fatalf("self delete error = %v", err)
	}
	if err := store.Create("second", "second-password", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := store.Update("second", "admin", UserPatch{Enabled: &disabled}); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("disable device owner error = %v", err)
	}
	if _, err := store.Update("second", "admin", UserPatch{Role: &userRole}); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("demote device owner error = %v", err)
	}
	if err := store.Delete("second", "admin"); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("delete device owner error = %v", err)
	}
	if err := store.Delete("admin", "second"); err != nil {
		t.Fatal(err)
	}
}

func TestRemovedAccountFileAndRecreatedUserInvalidateOldVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	store := seededStore(t, path)
	admin, ok, err := store.Authenticate("admin", testSeedPassword)
	if err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ValidateToken("admin", admin.TokenVersion); err == nil {
		t.Fatal("old admin token survived account-file reset")
	}

	if _, ok, err = store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("login after reset: ok=%v err=%v", ok, err)
	}
	if err = store.Create("alice", "alice-password", RoleUser); err != nil {
		t.Fatal(err)
	}
	alice, err := store.Get("alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Delete("admin", "alice"); err != nil {
		t.Fatal(err)
	}
	if err = store.Create("alice", "alice-password", RoleUser); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ValidateToken("alice", alice.TokenVersion); err == nil {
		t.Fatal("old token survived deletion and username reuse")
	}
}

func TestConcurrentCreatesDoNotLoseUsers(t *testing.T) {
	store := seededStore(t, filepath.Join(t.TempDir(), "pwd"))
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}

	const count = 5
	var workers sync.WaitGroup
	errorsCh := make(chan error, count)
	for index := 0; index < count; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			errorsCh <- store.Create(fmt.Sprintf("user%d", index), "valid-password", RoleUser)
		}(index)
	}
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	users, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != count+1 {
		t.Fatalf("got %d users, want %d", len(users), count+1)
	}
}

func TestLegacyReversiblePasswordIsRetired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	// what older releases stored: a reversibly encrypted password, not bcrypt
	legacy, _ := json.Marshal(legacyAccount{Username: "owner", Password: "U2FsdGVkX1+ZmFrZQ=="})
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(path)
	if _, ok, err := store.Authenticate("owner", "U2FsdGVkX1+ZmFrZQ=="); err != nil || ok {
		t.Fatalf("the legacy value must not log in: ok=%v err=%v", ok, err)
	}
	user, err := store.Get("owner")
	if err != nil {
		t.Fatal(err)
	}
	if !user.MustChangePassword || !isBcryptHash(user.PasswordHash) {
		t.Fatalf("legacy account must be re-seeded and forced to change: %+v", user)
	}
	// the owner can recover with the password on the device screen
	seed, err := os.ReadFile(store.seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Authenticate("owner", strings.TrimSpace(string(seed))); err != nil || !ok {
		t.Fatalf("seed password must log in: ok=%v err=%v", ok, err)
	}
}

// bootSeededStore returns a store whose boot-partition seed file holds content.
func bootSeededStore(t *testing.T, content string) *Store {
	t.Helper()
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "pwd"))
	store.bootSeedPath = filepath.Join(dir, "boot-initial.pass")
	if err := os.WriteFile(store.bootSeedPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestBootSeedSeedsOwnerAndIsConsumed(t *testing.T) {
	const want = "operator-chosen-seed"
	store := bootSeededStore(t, want+"\n")

	seeded, err := store.Init()
	if err != nil || seeded != want {
		t.Fatalf("Init seeded %q err=%v, want the boot seed", seeded, err)
	}
	if _, err = os.Stat(store.bootSeedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("boot seed must be removed once consumed: %v", err)
	}
	data, err := os.ReadFile(store.seedPath)
	if err != nil || strings.TrimSpace(string(data)) != want {
		t.Fatalf("seed file holds %q, err=%v", data, err)
	}
	user, ok, err := store.Authenticate("admin", want)
	if err != nil || !ok || !user.MustChangePassword {
		t.Fatalf("boot seed login: user=%+v ok=%v err=%v", user, ok, err)
	}
}

func TestBootSeedWithoutNewlineAndCRLF(t *testing.T) {
	for name, content := range map[string]string{"bare": "abcdefghijkl", "crlf": "abcdefghijkl\r\n"} {
		store := bootSeededStore(t, content)
		seeded, err := store.Init()
		if err != nil || seeded != "abcdefghijkl" {
			t.Fatalf("%s: seeded %q err=%v", name, seeded, err)
		}
	}
}

func TestShortBootSeedIsAccepted(t *testing.T) {
	store := bootSeededStore(t, "a\n")
	seeded, err := store.Init()
	if err != nil || seeded != "a" {
		t.Fatalf("a one-character boot seed must be used: seeded %q err=%v", seeded, err)
	}
	if _, err = os.Stat(store.bootSeedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("boot seed must be removed once consumed: %v", err)
	}
}

func TestEmptyOrOversizeBootSeedIsIgnored(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "newline": "\n", "crlf": "\r\n", "over72": strings.Repeat("x", 73) + "\n"} {
		store := bootSeededStore(t, content)
		seeded, err := store.Init()
		if err != nil || len(seeded) != seedPasswordLength {
			t.Fatalf("%s: must fall back to a random seed, got %q err=%v", name, seeded, err)
		}
		if _, err = os.Stat(store.bootSeedPath); err != nil {
			t.Fatalf("%s: an ignored boot seed is left in place: %v", name, err)
		}
	}
}

func TestValidatePasswordHasOnlySafetyBounds(t *testing.T) {
	for _, ok := range []string{"a", "1234", strings.Repeat("x", 72)} {
		if err := ValidatePassword(ok); err != nil {
			t.Errorf("ValidatePassword(%d bytes) = %v, want nil", len(ok), err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 73)} {
		if err := ValidatePassword(bad); err == nil {
			t.Errorf("ValidatePassword(%d bytes) accepted", len(bad))
		}
	}
	store := NewStore(filepath.Join(t.TempDir(), "pwd"))
	if err := store.Create("short", "a", RoleUser); err != nil {
		t.Fatalf("Create with a one-character password: %v", err)
	}
	if err := store.Create("empty", "", RoleUser); err == nil {
		t.Fatal("Create with an empty password must fail")
	}
}

// rootPassStore returns a store whose boot-partition root.pass holds content
// (no file when content is nil).
func rootPassStore(t *testing.T, content *string) *Store {
	t.Helper()
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "pwd"))
	store.bootRootPath = filepath.Join(dir, "boot-root.pass")
	if content != nil {
		if err := os.WriteFile(store.bootRootPath, []byte(*content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestRootPassOverridesOwnerSeedAndIsConsumed(t *testing.T) {
	content := "r\r\n"
	store := rootPassStore(t, &content)
	var got []string
	err := store.ApplyRootPassword("owner-seed", func(p string) error { got = append(got, p); return nil })
	if err != nil || len(got) != 1 || got[0] != "r" {
		t.Fatalf("root set to %q err=%v, want only root.pass", got, err)
	}
	if _, err = os.Stat(store.bootRootPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root.pass must be removed once applied: %v", err)
	}
}

func TestAbsentRootPassUsesOwnerSeed(t *testing.T) {
	store := rootPassStore(t, nil)
	var got []string
	err := store.ApplyRootPassword("owner-seed", func(p string) error { got = append(got, p); return nil })
	if err != nil || len(got) != 1 || got[0] != "owner-seed" {
		t.Fatalf("root set to %q err=%v, want the owner seed", got, err)
	}
}

func TestEmptyRootPassUsesOwnerSeedAndStays(t *testing.T) {
	content := "\n"
	store := rootPassStore(t, &content)
	var got []string
	err := store.ApplyRootPassword("owner-seed", func(p string) error { got = append(got, p); return nil })
	if err != nil || len(got) != 1 || got[0] != "owner-seed" {
		t.Fatalf("root set to %q err=%v, want the owner seed", got, err)
	}
	if _, err = os.Stat(store.bootRootPath); err != nil {
		t.Fatalf("an ignored root.pass is left in place: %v", err)
	}
}

func TestRefusedRootPassFallsBackToOwnerSeedAndStays(t *testing.T) {
	content := "bad:pass\n"
	store := rootPassStore(t, &content)
	var got []string
	err := store.ApplyRootPassword("owner-seed", func(p string) error {
		got = append(got, p)
		if p == "bad:pass" {
			return errors.New("refused")
		}
		return nil
	})
	if err != nil || len(got) != 2 || got[1] != "owner-seed" {
		t.Fatalf("set calls %q err=%v, want root.pass refused then the owner seed", got, err)
	}
	if _, err = os.Stat(store.bootRootPath); err != nil {
		t.Fatalf("a refused root.pass is left in place: %v", err)
	}
}

func TestBootSeedDoesNotOverrideExistingAccountOrSeedFile(t *testing.T) {
	store := bootSeededStore(t, "operator-chosen-seed\n")
	if err := os.WriteFile(store.seedPath, []byte("button-reset-seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seeded, err := store.Init()
	if err != nil || seeded != "button-reset-seed" {
		t.Fatalf("an existing seed file wins: seeded %q err=%v", seeded, err)
	}
	if _, err = os.Stat(store.bootSeedPath); err != nil {
		t.Fatalf("boot seed not consumed, so it stays: %v", err)
	}

	// the account exists now: a boot seed dropped later changes nothing
	if err = os.WriteFile(store.bootSeedPath, []byte("late-operator-seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seeded, err = store.Init()
	if err != nil || seeded != "" {
		t.Fatalf("second Init: seeded=%q err=%v, want empty", seeded, err)
	}
	if _, ok, err := store.Authenticate("admin", "late-operator-seed"); err != nil || ok {
		t.Fatalf("late boot seed must not become a password: ok=%v err=%v", ok, err)
	}
}

func TestUnreadableBootSeedFallsBackToRandom(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "pwd"))
	store.bootSeedPath = filepath.Join(dir, "boot-initial.pass")
	if err := os.Mkdir(store.bootSeedPath, 0o755); err != nil { // reading a directory fails
		t.Fatal(err)
	}
	seeded, err := store.Init()
	if err != nil || len(seeded) != seedPasswordLength {
		t.Fatalf("seeded %q err=%v, want a random seed", seeded, err)
	}
}

func TestAbsentBootSeedKeepsRandomSeed(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "pwd"))
	store.bootSeedPath = filepath.Join(dir, "missing")
	seeded, err := store.Init()
	if err != nil || len(seeded) != seedPasswordLength {
		t.Fatalf("seeded %q err=%v, want a random seed", seeded, err)
	}
}
