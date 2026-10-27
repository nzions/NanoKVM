package authn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"NanoKVM-Server/utils"
)

// oldAccountFile is a 2.5.15 account store: version 1, the owner "admin" as
// the system account with a bcrypt hash of a password the web no longer
// needs, plus a second user that must not survive.
const oldAccountFile = `{
  "version": 1,
  "users": [
    {"username": "admin", "password": "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234", "role": "admin", "enabled": true, "tokenVersion": 7, "mustChangePassword": %s, "systemAccount": true},
    {"username": "alice", "password": "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234", "role": "user", "enabled": true, "tokenVersion": 9}
  ],
  "username": "admin",
  "password": "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"
}
`

type fixture struct {
	dir    string
	shadow string
	store  *Store
}

func newFixture(t *testing.T, rootHash string) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{dir: dir, shadow: filepath.Join(dir, "shadow")}
	original, originalRoot := utils.ShadowFile, utils.RootPassFile
	utils.ShadowFile, utils.RootPassFile = f.shadow, filepath.Join(dir, "root.pass")
	t.Cleanup(func() { utils.ShadowFile, utils.RootPassFile = original, originalRoot })
	if err := os.WriteFile(f.shadow, []byte("daemon:*:19000:0:99999:7:::\nroot:"+rootHash+":19000:0:99999:7:::\nnik:$6$x$y:19001:::::\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.store = NewStore(filepath.Join(dir, "pwd"))
	f.store.bootRootPath = filepath.Join(dir, "boot-root.pass")
	return f
}

func (f *fixture) write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(f.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *fixture) exists(name string) bool {
	_, err := os.Stat(filepath.Join(f.dir, name))
	return err == nil
}

func (f *fixture) readState(t *testing.T) state {
	t.Helper()
	data, err := os.ReadFile(f.store.path)
	if err != nil {
		t.Fatal(err)
	}
	var st state
	if err = json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func mustAuth(t *testing.T, s *Store, user, password string) (*User, bool) {
	t.Helper()
	u, ok, err := s.Authenticate(user, password)
	if err != nil {
		t.Fatalf("Authenticate(%q): %v", user, err)
	}
	return u, ok
}

func TestLoginAsRootAgainstEveryStockHashType(t *testing.T) {
	for name, hash := range map[string]string{
		"sha512": utils.SHA512Crypt("pw-one", "saltsalt", 0),
		"sha256": utils.SHA256Crypt("pw-one", "saltsalt", 0),
		"md5":    utils.MD5Crypt("pw-one", "saltsalt"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, hash)
			f.write(t, "pwd", `{"version": 2, "tokenVersion": 5}`)
			user, ok := mustAuth(t, f.store, "root", "pw-one")
			if !ok || user.Username != "root" || user.Role != RoleAdmin || user.TokenVersion != 5 {
				t.Fatalf("root login = %+v, %v", user, ok)
			}
			if _, ok = mustAuth(t, f.store, "root", "pw-two"); ok {
				t.Fatal("wrong password accepted")
			}
			if _, ok = mustAuth(t, f.store, "admin", "pw-one"); ok {
				t.Fatal("login as admin accepted")
			}
			if _, ok = mustAuth(t, f.store, "root", ""); ok {
				t.Fatal("empty password accepted")
			}
			if _, ok = mustAuth(t, f.store, "root", strings.Repeat("a", MaxPasswordBytes+1)); ok {
				t.Fatal("over-long password accepted")
			}
		})
	}
}

func TestAuthenticateUnverifiableHashIsAnError(t *testing.T) {
	f := newFixture(t, "abJnggxhB/yWI") // DES
	f.write(t, "pwd", `{"version": 2, "tokenVersion": 5}`)
	if _, ok, err := f.store.Authenticate("root", "x"); ok || err == nil {
		t.Fatalf("DES hash: ok=%v err=%v, want an error", ok, err)
	}
}

func rootHashOf(t *testing.T) string {
	t.Helper()
	hash, err := utils.RootHash()
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestFirstBootWithoutRootPassLocksRootAndLeavesPasswordUnset(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if hash := rootHashOf(t); hash != "!" {
		t.Fatalf("root's shadow hash = %q, want the lock %q", hash, "!")
	}
	if f.exists("initial.pass") {
		t.Fatal("a seed password file was written")
	}
	if st := f.readState(t); !st.PasswordUnset || st.MustChangePassword {
		t.Fatalf("state = %+v, want unset and no forced change", st)
	}
	if set, err := f.store.PasswordSet(); err != nil || set {
		t.Fatalf("PasswordSet = %v, %v; want false", set, err)
	}
	// neither the stock password nor anything else logs in
	for _, password := range []string{"root", "x", "!"} {
		if _, ok, err := f.store.Authenticate("root", password); ok || !errors.Is(err, ErrPasswordUnset) {
			t.Fatalf("login %q while unset: ok=%v err=%v, want ErrPasswordUnset", password, ok, err)
		}
	}
	// only root's line changed
	data, _ := os.ReadFile(f.shadow)
	if !strings.Contains(string(data), "nik:$6$x$y:19001:::::\n") || !strings.Contains(string(data), "daemon:*:19000:0:99999:7:::\n") {
		t.Fatalf("other shadow lines changed:\n%s", data)
	}

	// a second Init changes nothing
	before := f.readState(t)
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if after := f.readState(t); after != before {
		t.Fatalf("state changed on second Init: %+v -> %+v", before, after)
	}
}

func TestSetupThenLoginWorksAndSecondSetupIsRefused(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	f.write(t, "initial.pass", "left-by-an-old-release\n")
	f.write(t, "root.pass", "stale-rotated-root\n")
	if _, err := f.store.SetupPassword(""); err == nil {
		t.Fatal("empty password accepted by setup")
	}
	if _, err := f.store.SetupPassword("a:b"); err == nil {
		t.Fatal("password with a colon accepted by setup")
	}
	if set, _ := f.store.PasswordSet(); set {
		t.Fatal("a refused setup cleared the unset state")
	}

	user, err := f.store.SetupPassword("first-connection-pw")
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordUnset || user.MustChangePassword {
		t.Fatalf("after setup: %+v", user)
	}
	if ok, err := utils.VerifyRootPassword("first-connection-pw"); err != nil || !ok {
		t.Fatalf("shadow not updated: ok=%v err=%v", ok, err)
	}
	if f.exists("initial.pass") || f.exists("root.pass") {
		t.Fatal("seed or rotated password file survived the setup")
	}
	if set, err := f.store.PasswordSet(); err != nil || !set {
		t.Fatalf("PasswordSet = %v, %v; want true", set, err)
	}
	logged, ok := mustAuth(t, f.store, "root", "first-connection-pw")
	if !ok || logged.TokenVersion != user.TokenVersion {
		t.Fatalf("login = %+v, %v; want the setup session's version %d", logged, ok, user.TokenVersion)
	}
	if _, ok = mustAuth(t, f.store, "root", "root"); ok {
		t.Fatal("the stock password works")
	}

	// never a second setup, and it must not change the password
	if _, err = f.store.SetupPassword("attacker-pw"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Fatalf("second setup = %v, want ErrPasswordAlreadySet", err)
	}
	if ok, _ := utils.VerifyRootPassword("first-connection-pw"); !ok {
		t.Fatal("a refused second setup changed the password")
	}
}

func TestConcurrentSetupsHaveExactlyOneWinner(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	const callers = 8
	var wg sync.WaitGroup
	var wins, refused atomic.Int32
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.store.SetupPassword(fmt.Sprintf("password-%d", i))
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrPasswordAlreadySet):
				refused.Add(1)
			default:
				t.Errorf("setup %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 || refused.Load() != callers-1 {
		t.Fatalf("wins=%d refused=%d, want 1 and %d", wins.Load(), refused.Load(), callers-1)
	}
}

func TestStateFromAnEarlierReleaseIsUnaffected(t *testing.T) {
	// a device whose password is set (no passwordUnset field) stays set, and its
	// old must-change-from-seed flag keeps working
	f := newFixture(t, utils.SHA512Crypt("pw-one", "saltsalt", 0))
	f.write(t, "pwd", `{"version": 2, "tokenVersion": 5, "mustChangePassword": true}`)
	user, ok := mustAuth(t, f.store, "root", "pw-one")
	if !ok || !user.MustChangePassword || user.PasswordUnset {
		t.Fatalf("login = %+v, %v; want must-change and not unset", user, ok)
	}
	if set, err := f.store.PasswordSet(); err != nil || !set {
		t.Fatalf("PasswordSet = %v, %v; want true", set, err)
	}
	if _, err := f.store.SetupPassword("attacker-pw"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Fatalf("setup on a set device = %v, want ErrPasswordAlreadySet", err)
	}
	if ok, _ := utils.VerifyRootPassword("pw-one"); !ok {
		t.Fatal("setup on a set device changed the password")
	}
}

func TestBootRootPassWinsAndIsConsumed(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	bootFile := f.write(t, "boot-root.pass", "from-the-card\r\n")
	f.write(t, "initial.pass", "stale-seed-file\n")
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if ok, err := utils.VerifyRootPassword("from-the-card"); err != nil || !ok {
		t.Fatalf("root.pass password not applied: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(bootFile); !os.IsNotExist(err) {
		t.Fatalf("root.pass was not consumed: %v", err)
	}
	if f.exists("initial.pass") {
		t.Fatal("stale seed file kept beside an operator-chosen password")
	}
	if st := f.readState(t); st.PasswordUnset {
		t.Fatalf("state = %+v, root.pass must leave the password set", st)
	}
	user, ok := mustAuth(t, f.store, "root", "from-the-card")
	if !ok || user.MustChangePassword {
		t.Fatalf("login = %+v, %v; an operator-chosen password must not force a change", user, ok)
	}
	if _, err := f.store.SetupPassword("attacker-pw"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Fatalf("setup after root.pass = %v, want ErrPasswordAlreadySet", err)
	}
}

func TestUnusableBootRootPassLeavesRootLockedAndUnset(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	bootFile := f.write(t, "boot-root.pass", "has:colon\n")
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bootFile); err != nil {
		t.Fatalf("an unusable root.pass should be left alone: %v", err)
	}
	if hash := rootHashOf(t); hash != "!" {
		t.Fatalf("root's shadow hash = %q, want the lock", hash)
	}
	if set, _ := f.store.PasswordSet(); set {
		t.Fatal("password reported set")
	}
}

func TestButtonResetStateGoesBackToUnset(t *testing.T) {
	// the button reset locks root and removes the state: the next use is unset
	f := newFixture(t, utils.SHA512Crypt("old", "saltsalt", 0))
	if _, err := f.store.SetPassword("old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.store.path); err != nil {
		t.Fatal(err)
	}
	if set, err := f.store.PasswordSet(); err != nil || set {
		t.Fatalf("PasswordSet after reset = %v, %v; want false", set, err)
	}
	if _, ok, err := f.store.Authenticate("root", "old"); ok || !errors.Is(err, ErrPasswordUnset) {
		t.Fatalf("login after reset: ok=%v err=%v", ok, err)
	}
}

func TestSetPasswordChangesRootAndRevokes(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	first, err := f.store.SetupPassword("initial-pass")
	if err != nil {
		t.Fatal(err)
	}
	f.write(t, "root.pass", "rotated\n")

	if _, err := f.store.SetPassword(""); err == nil {
		t.Fatal("empty password accepted")
	}
	if _, err := f.store.SetPassword("a:b"); err == nil {
		t.Fatal("password with a colon accepted")
	}
	if _, err := f.store.SetPassword(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Fatal("over-long password accepted")
	}
	changed, err := f.store.SetPassword("brand-new-pass")
	if err != nil {
		t.Fatal(err)
	}
	if changed.MustChangePassword || changed.TokenVersion == first.TokenVersion {
		t.Fatalf("after change: %+v (before %+v)", changed, first)
	}
	if ok, err := utils.VerifyRootPassword("brand-new-pass"); err != nil || !ok {
		t.Fatalf("shadow not updated: ok=%v err=%v", ok, err)
	}
	if _, ok := mustAuth(t, f.store, "root", "initial-pass"); ok {
		t.Fatal("old password still works")
	}
	if f.exists("root.pass") {
		t.Fatal("rotated password file survived the owner's change")
	}
	if _, err = f.store.ValidateToken("root", first.TokenVersion); err == nil {
		t.Fatal("pre-change session still valid")
	}
	if _, err = f.store.ValidateToken("root", changed.TokenVersion); err != nil {
		t.Fatalf("post-change session: %v", err)
	}
	// only root's line changed
	data, _ := os.ReadFile(f.shadow)
	if !strings.Contains(string(data), "nik:$6$x$y:19001:::::\n") || !strings.Contains(string(data), "daemon:*:19000:0:99999:7:::\n") {
		t.Fatalf("other shadow lines changed:\n%s", data)
	}
}

func TestRevokeAndValidateToken(t *testing.T) {
	f := newFixture(t, utils.SHA512Crypt("pw", "saltsalt", 0))
	f.write(t, "pwd", `{"version": 2, "tokenVersion": 11}`)
	if _, err := f.store.ValidateToken("root", 11); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ValidateToken("admin", 11); err == nil {
		t.Fatal("token for admin validated")
	}
	if _, err := f.store.Revoke("root"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ValidateToken("root", 11); err == nil {
		t.Fatal("revoked token still valid")
	}
	// removing the state (the button reset) invalidates sessions until re-seeded
	if err := os.Remove(f.store.path); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ValidateToken("root", 12); err == nil {
		t.Fatal("token valid without state")
	}
}

func TestMigrationFrom2515KeepsRootsShadow(t *testing.T) {
	for _, mustChange := range []string{"false", "true"} {
		t.Run("mustChange="+mustChange, func(t *testing.T) {
			hash := utils.SHA512Crypt("the-web-admin-pass", "saltsalt", 0) // copied into shadow by 2.5.14+
			f := newFixture(t, hash)
			f.write(t, "pwd", strings.Replace(oldAccountFile, "%s", mustChange, 1))
			shadowBefore, _ := os.ReadFile(f.shadow)

			if err := f.store.Init(); err != nil {
				t.Fatal(err)
			}
			if shadowAfter, _ := os.ReadFile(f.shadow); string(shadowAfter) != string(shadowBefore) {
				t.Fatal("migration touched /etc/shadow")
			}
			st := f.readState(t)
			if st.Version != stateVersion || st.TokenVersion == 0 || st.MustChangePassword != (mustChange == "true") {
				t.Fatalf("state = %+v", st)
			}
			data, _ := os.ReadFile(f.store.path)
			if strings.Contains(string(data), "$2a$") || strings.Contains(string(data), "alice") {
				t.Fatalf("old accounts survived the migration:\n%s", data)
			}
			if user, ok := mustAuth(t, f.store, "root", "the-web-admin-pass"); !ok || user.MustChangePassword != (mustChange == "true") {
				t.Fatalf("root login after upgrade = %+v, %v", user, ok)
			}
			if _, ok := mustAuth(t, f.store, "admin", "the-web-admin-pass"); ok {
				t.Fatal("admin can still log in")
			}
			if _, err := f.store.ValidateToken("admin", 7); err == nil {
				t.Fatal("an old admin session survived")
			}
		})
	}
}

func TestMigrationFromSingleAccountFile(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("kept", "saltsalt"))
	f.write(t, "pwd", `{"username": "admin", "password": "U2FsdGVkX1+legacy"}`)
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustAuth(t, f.store, "root", "kept"); !ok {
		t.Fatal("root locked out after migrating the single-account file")
	}
}

func TestMigrationRefusedWhenRootHashIsUnverifiable(t *testing.T) {
	for name, hash := range map[string]string{
		"locked":   "!",
		"star":     "*",
		"empty":    "",
		"des":      "abJnggxhB/yWI",
		"yescrypt": "$y$j9T$saltsalt$hashhashhash",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, hash)
			old := strings.Replace(oldAccountFile, "%s", "false", 1)
			path := f.write(t, "pwd", old)
			shadowBefore, _ := os.ReadFile(f.shadow)

			if err := f.store.Init(); err == nil {
				t.Fatal("migration went ahead with a hash the web cannot verify")
			}
			if _, _, err := f.store.Authenticate("root", "x"); err == nil {
				t.Fatal("Authenticate answered although migration was refused")
			}
			if data, _ := os.ReadFile(path); string(data) != old {
				t.Fatal("the old account file was changed or removed")
			}
			if shadowAfter, _ := os.ReadFile(f.shadow); string(shadowAfter) != string(shadowBefore) {
				t.Fatal("a refused migration touched /etc/shadow")
			}
		})
	}
}

func TestMigrationRefusedWhenShadowHasNoRoot(t *testing.T) {
	f := newFixture(t, "!")
	if err := os.WriteFile(f.shadow, []byte("daemon:*:19000:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := f.write(t, "pwd", strings.Replace(oldAccountFile, "%s", "false", 1))
	if err := f.store.Init(); err == nil {
		t.Fatal("migration went ahead without a root entry")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("old account file lost: %v", err)
	}
}

func TestUnknownOrBrokenStateIsNeverOverwritten(t *testing.T) {
	for name, content := range map[string]string{
		"newer version": `{"version": 3, "tokenVersion": 1}`,
		"not json":      `garbage`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, utils.SHA512Crypt("pw", "saltsalt", 0))
			path := f.write(t, "pwd", content)
			if err := f.store.Init(); err == nil {
				t.Fatal("Init accepted it")
			}
			if data, _ := os.ReadFile(path); string(data) != content {
				t.Fatal("file was overwritten")
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	for _, ok := range []string{"a", "short", strings.Repeat("x", MaxPasswordBytes), "päss wörd ✓"} {
		if err := ValidatePassword(ok); err != nil {
			t.Errorf("ValidatePassword(%.20q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", MaxPasswordBytes+1), "a:b", "a\nb", "a\x00b"} {
		if err := ValidatePassword(bad); err == nil {
			t.Errorf("ValidatePassword(%.20q) accepted", bad)
		}
	}
}
