package authn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestFirstBootGeneratesSeedForRootAndForcesChange(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(f.store.seedPath)
	if err != nil {
		t.Fatalf("seed file: %v", err)
	}
	password := strings.TrimSpace(string(seed))
	if len(password) != seedPasswordLength {
		t.Fatalf("seed %q is not %d characters", password, seedPasswordLength)
	}
	if info, _ := os.Stat(f.store.seedPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("seed file mode = %v", info.Mode().Perm())
	}
	if ok, err := utils.VerifyRootPassword(password); err != nil || !ok {
		t.Fatalf("root's shadow password is not the seed: ok=%v err=%v", ok, err)
	}
	user, ok := mustAuth(t, f.store, "root", password)
	if !ok || !user.MustChangePassword {
		t.Fatalf("seed login = %+v, %v; want must-change", user, ok)
	}
	if _, ok = mustAuth(t, f.store, "root", "root"); ok {
		t.Fatal("the stock password still works")
	}

	// a second Init changes nothing
	before := f.readState(t)
	if err = f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if after := f.readState(t); after != before {
		t.Fatalf("state changed on second Init: %+v -> %+v", before, after)
	}
}

func TestFirstBootReusesSeedFile(t *testing.T) {
	// the button reset writes initial.pass and removes the state
	f := newFixture(t, utils.SHA512Crypt("old", "saltsalt", 0))
	f.write(t, "initial.pass", "Reset12345Ab\n")
	f.write(t, "root.pass", "stale-rotated-root\n")
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := utils.VerifyRootPassword("Reset12345Ab"); !ok {
		t.Fatal("root did not take the seed file's password")
	}
	if f.exists("root.pass") {
		t.Fatal("the rotated root.pass was not cleared")
	}
	if !f.exists("initial.pass") {
		t.Fatal("seed file removed before the owner chose a password")
	}
}

func TestBootRootPassSeedsRootAndIsConsumed(t *testing.T) {
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
	user, ok := mustAuth(t, f.store, "root", "from-the-card")
	if !ok || user.MustChangePassword {
		t.Fatalf("login = %+v, %v; an operator-chosen password must not force a change", user, ok)
	}
}

func TestBootRootPassRefusedByShadowFallsBackToSeed(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	bootFile := f.write(t, "boot-root.pass", "has:colon\n")
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bootFile); err != nil {
		t.Fatalf("an unusable root.pass should be left alone: %v", err)
	}
	if ok, _ := utils.VerifyRootPassword("has:colon"); ok {
		t.Fatal("unusable root.pass was applied")
	}
	seed, err := os.ReadFile(f.store.seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := utils.VerifyRootPassword(strings.TrimSpace(string(seed))); !ok {
		t.Fatal("root did not fall back to the seed")
	}
}

func TestSetPasswordChangesRootAndRevokes(t *testing.T) {
	f := newFixture(t, utils.MD5Crypt("root", "stocksal"))
	if err := f.store.Init(); err != nil {
		t.Fatal(err)
	}
	seed, _ := os.ReadFile(f.store.seedPath)
	first, ok := mustAuth(t, f.store, "root", strings.TrimSpace(string(seed)))
	if !ok {
		t.Fatal("seed login failed")
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
	if ok, err = utils.VerifyRootPassword("brand-new-pass"); err != nil || !ok {
		t.Fatalf("shadow not updated: ok=%v err=%v", ok, err)
	}
	if _, ok = mustAuth(t, f.store, "root", string(seed[:len(seed)-1])); ok {
		t.Fatal("seed password still works")
	}
	if f.exists("initial.pass") || f.exists("root.pass") {
		t.Fatal("seed or rotated password file survived the owner's change")
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
