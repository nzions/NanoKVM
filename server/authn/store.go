// Package authn is the web login. There is exactly one account, root, and its
// password is root's Linux password: the hash in /etc/shadow is the only copy,
// checked by utils.VerifyRootPassword (crypt $6$, $5$ and $1$). The state file
// next to the old account store holds only what the shadow file cannot: the
// session generation (bumped to revoke every session) and whether the next
// login must change the password.
package authn

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"NanoKVM-Server/utils"

	log "github.com/sirupsen/logrus"
)

const (
	// AccountFile is the session state file. Releases up to 2.5.15 kept the
	// multi-user account store (bcrypt hashes) here; it is replaced by the
	// state below the first time this release starts.
	AccountFile = "/etc/kvm/pwd"
	// SeedPasswordFile holds the first-boot (or button-reset) password. It is
	// 0600 and shown on the device's own screen by kvm_system; it is removed once
	// the owner changes the password.
	SeedPasswordFile = "/etc/kvm/initial.pass"
	// BootRootPassFile is an optional operator-supplied root password on the FAT
	// boot partition (writable from a Mac). At first boot it becomes root's
	// password, is not marked for change, and is removed once applied.
	BootRootPassFile = "/boot/root.pass"

	// Username is the one account.
	Username = "root"

	stateVersion = 2
	// MaxPasswordBytes bounds a password. crypt hashes the password thousands of
	// times, so an unbounded one sent to the unauthenticated login route would be
	// a CPU sink on this small SoC. (The bcrypt-era 72-byte limit is gone:
	// SHA-crypt has no such limit, and the old passwords were all within it.)
	MaxPasswordBytes   = 256
	seedPasswordLength = 12
	// seedPasswordAlphabet has no look-alike characters (0/O, 1/l/I).
	seedPasswordAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
)

type Role string

// RoleAdmin is the role of the one account; every protected route requires it.
const RoleAdmin Role = "admin"

var (
	// ErrNotInitialized is returned by read-only calls when there is no usable
	// state yet; only Init and Authenticate create or migrate it.
	ErrNotInitialized = errors.New("account state not initialized")
	// ErrWrongUser is returned for any username other than root.
	ErrWrongUser = errors.New("user not found")
)

// User is the root account as the web sees it.
type User struct {
	Username           string
	Role               Role
	TokenVersion       uint64
	MustChangePassword bool
}

// state is the file at Store.path.
type state struct {
	Version            int    `json:"version"`
	TokenVersion       uint64 `json:"tokenVersion"`
	MustChangePassword bool   `json:"mustChangePassword,omitempty"`
}

// legacyFile reads the account store of releases up to 2.5.15 (version 1, a
// users list) or the older single-account form. Only the owner's
// must-change flag is carried over; no hash is, because root's shadow entry
// already holds the password those releases kept in step.
type legacyFile struct {
	Version int          `json:"version"`
	Users   []legacyUser `json:"users"`
}

type legacyUser struct {
	Username           string `json:"username"`
	SystemAccount      bool   `json:"systemAccount"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

type Store struct {
	path     string
	seedPath string
	// bootRootPath is the operator-supplied root password on the boot partition.
	bootRootPath string
	mutex        sync.RWMutex
}

var DefaultStore = NewStore(AccountFile)

// NewStore keeps the seed password file next to the state file.
func NewStore(path string) *Store {
	return &Store{
		path:         path,
		seedPath:     filepath.Join(filepath.Dir(path), filepath.Base(SeedPasswordFile)),
		bootRootPath: BootRootPassFile,
	}
}

// ValidatePassword has no policy: any non-empty password is accepted (an empty
// one would mean no authentication). Bounds: MaxPasswordBytes, and none of
// NUL, ':', CR, LF, which the shadow update refuses.
func ValidatePassword(password string) error {
	if password == "" {
		return errors.New("password must not be empty")
	}
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("password must be at most %d bytes", MaxPasswordBytes)
	}
	if strings.ContainsAny(password, "\r\n:\x00") {
		return errors.New("password must not contain a colon, a newline or a NUL")
	}
	return nil
}

// Init creates the state when missing: it migrates a 2.5.15-style account file
// or seeds root from the boot partition or a generated password. It is also
// run lazily by Authenticate, so deleting the state file (the button reset
// does) re-seeds a running server.
func (s *Store) Init() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, err := s.loadLocked(true)
	return err
}

// Get returns the account. It never creates or migrates state.
func (s *Store) Get(username string) (*User, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.getLocked(username)
}

func (s *Store) getLocked(username string) (*User, error) {
	if username != Username {
		return nil, ErrWrongUser
	}
	st, err := s.loadLocked(false)
	if err != nil {
		return nil, err
	}
	return userOf(st), nil
}

// Authenticate checks the password against root's /etc/shadow hash. A wrong
// user or password is (nil, false, nil); an error means the answer is unknown
// (state unreadable, or root's hash is of a type that cannot be checked).
func (s *Store) Authenticate(username, password string) (*User, bool, error) {
	if username != Username || password == "" || len(password) > MaxPasswordBytes {
		return nil, false, nil
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	st, err := s.loadLocked(true)
	if err != nil {
		return nil, false, err
	}
	ok, err := utils.VerifyRootPassword(password)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return userOf(st), true, nil
}

func (s *Store) ValidateToken(username string, tokenVersion uint64) (*User, error) {
	user, err := s.Get(username)
	if err != nil {
		return nil, err
	}
	if user.TokenVersion != tokenVersion {
		return nil, errors.New("session revoked")
	}
	return user, nil
}

// SetPassword makes password root's Linux password (and so the web one),
// clears the must-change flag, revokes every session and drops the seed file.
// The shadow file is written first: if that fails nothing changed.
func (s *Store) SetPassword(password string) (*User, error) {
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	st, err := s.loadLocked(true)
	if err != nil {
		return nil, err
	}
	if err = utils.SetRootPassword(password); err != nil {
		return nil, err
	}
	st.MustChangePassword = false
	st.TokenVersion++
	if err = s.saveLocked(st); err != nil {
		return nil, fmt.Errorf("root password changed but the session state could not be saved: %w", err)
	}
	// the owner chose a password: neither the seed nor a generated root one is
	// shown on the screen any more
	if rmErr := os.Remove(s.seedPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		log.Errorf("failed to remove %s: %v", s.seedPath, rmErr)
	}
	if rmErr := utils.ClearRootPassFile(); rmErr != nil {
		log.Errorf("failed to remove %s: %v", utils.RootPassFile, rmErr)
	}
	return userOf(st), nil
}

// Revoke invalidates every session.
func (s *Store) Revoke(username string) (*User, error) {
	if username != Username {
		return nil, ErrWrongUser
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	st, err := s.loadLocked(true)
	if err != nil {
		return nil, err
	}
	st.TokenVersion++
	if err = s.saveLocked(st); err != nil {
		return nil, err
	}
	return userOf(st), nil
}

func userOf(st *state) *User {
	return &User{
		Username:           Username,
		Role:               RoleAdmin,
		TokenVersion:       st.TokenVersion,
		MustChangePassword: st.MustChangePassword,
	}
}

// loadLocked reads the state. When ensure is set a missing file is seeded and a
// pre-2.5.16 account file is migrated; otherwise both are ErrNotInitialized.
// A file of an unknown (newer) version or that does not parse is an error and
// is never overwritten.
func (s *Store) loadLocked(ensure bool) (*state, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if !ensure {
			return nil, ErrNotInitialized
		}
		return s.seedLocked()
	}
	if err != nil {
		return nil, err
	}

	var head legacyFile
	if err = json.Unmarshal(data, &head); err != nil {
		return nil, errors.New("invalid account file")
	}
	if head.Version > stateVersion {
		return nil, fmt.Errorf("unsupported account file version: %d", head.Version)
	}
	if head.Version == stateVersion {
		var st state
		if err = json.Unmarshal(data, &st); err != nil {
			return nil, errors.New("invalid account file")
		}
		if st.TokenVersion == 0 {
			if st.TokenVersion, err = newTokenVersion(); err != nil {
				return nil, err
			}
			if ensure {
				if err = s.saveLocked(&st); err != nil {
					return nil, err
				}
			}
		}
		return &st, nil
	}
	if !ensure {
		return nil, ErrNotInitialized
	}
	return s.migrateLocked(data)
}

// migrateLocked replaces a pre-2.5.16 account file (the multi-user store, or
// the older single account) with the state. Root's shadow entry is not touched:
// since 2.5.14 the owner's web password was copied into it on every change, so
// it is the password the owner knows. It refuses, leaving the old file in
// place, unless that entry's hash is one the web can verify, so an upgraded
// device cannot end up with a login nothing can pass.
func (s *Store) migrateLocked(data []byte) (*state, error) {
	if err := utils.CheckRootHashVerifiable(); err != nil {
		log.Errorf("account migration refused, %s kept as it is: root's password in /etc/shadow cannot be verified by the web login (%v); reset the password on the device screen", s.path, err)
		return nil, fmt.Errorf("account migration refused: %w", err)
	}

	var old legacyFile
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, errors.New("invalid account file")
	}
	mustChange := false
	for _, user := range old.Users {
		if user.SystemAccount || user.Username == "admin" {
			mustChange = user.MustChangePassword
			break
		}
	}

	tokenVersion, err := newTokenVersion()
	if err != nil {
		return nil, err
	}
	st := &state{Version: stateVersion, TokenVersion: tokenVersion, MustChangePassword: mustChange}
	if err = s.saveLocked(st); err != nil {
		return nil, err
	}
	log.Infof("account store migrated: web login is now root with root's /etc/shadow password (must change: %v); other accounts dropped", mustChange)
	return st, nil
}

// seedLocked gives root its first password and creates the state. The boot
// partition's root.pass wins and is consumed once applied; it is a chosen
// password, so no change is forced. Otherwise root gets the seed from the seed
// file, or a generated one written there for the device screen, and the first
// login must change it. A root.pass that the shadow update refuses is logged
// and left, and the seed is used.
func (s *Store) seedLocked() (*state, error) {
	password, mustChange, fromBoot := "", true, false
	if boot, ok := readBootPassword(s.bootRootPath, "first-boot root password"); ok {
		if err := utils.SetRootPassword(boot); err == nil {
			password, mustChange, fromBoot = boot, false, true
		} else {
			log.Errorf("root password from %s could not be applied (%v): using the generated seed", s.bootRootPath, err)
		}
	}
	if !fromBoot {
		var err error
		if password, err = s.seedPassword(); err != nil {
			return nil, err
		}
		if err = utils.SetRootPassword(password); err != nil {
			return nil, fmt.Errorf("set root to the seed password: %w", err)
		}
	}

	tokenVersion, err := newTokenVersion()
	if err != nil {
		return nil, err
	}
	st := &state{Version: stateVersion, TokenVersion: tokenVersion, MustChangePassword: mustChange}
	if err = s.saveLocked(st); err != nil {
		return nil, err
	}
	if fromBoot {
		log.Infof("root password set from %s", s.bootRootPath)
		removeBootFile(s.bootRootPath, "first-boot root password")
		// a seed left from an earlier reset is not root's password
		if rmErr := os.Remove(s.seedPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			log.Errorf("failed to remove %s: %v", s.seedPath, rmErr)
		}
	}
	// root now has the seed or the chosen password: a rotated one is obsolete
	if err = utils.ClearRootPassFile(); err != nil {
		log.Errorf("failed to remove %s: %v", utils.RootPassFile, err)
	}
	return st, nil
}

// seedPassword is the password already in the seed file (written by the button
// reset or an interrupted first boot) or a fresh random one, written 0600 so
// the device screen can show it.
func (s *Store) seedPassword() (string, error) {
	if data, err := os.ReadFile(s.seedPath); err == nil {
		if password := strings.TrimSpace(string(data)); ValidatePassword(password) == nil {
			return password, nil
		}
	}
	password, err := randomPassword(seedPasswordLength)
	if err != nil {
		return "", err
	}
	if err = writeFile0600(s.seedPath, []byte(password+"\n")); err != nil {
		return "", err
	}
	return password, nil
}

func (s *Store) saveLocked(st *state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	directory := filepath.Dir(s.path)
	if err = os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".pwd-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temporaryPath, s.path); err != nil {
		return err
	}

	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

// readBootPassword reads a one-line password file from the boot partition: one
// trailing "\n" and then one "\r" are trimmed, and the rest must be a valid
// password. False when the file is absent or unusable; the reason is logged,
// never the value.
func readBootPassword(path, what string) (string, bool) {
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		log.Errorf("%s %s unreadable (%v): ignored", what, path, err)
		return "", false
	}
	password := strings.TrimSuffix(string(data), "\n")
	password = strings.TrimSuffix(password, "\r")
	if err = ValidatePassword(password); err != nil {
		log.Errorf("%s %s ignored: %v", what, path, err)
		return "", false
	}
	return password, true
}

// removeBootFile removes a boot-partition password file and syncs its directory.
func removeBootFile(path, what string) {
	if err := os.Remove(path); err != nil {
		log.Errorf("SECURITY: could not remove the %s %s (%v): the plaintext password stays on the boot partition until it is deleted by hand", what, path, err)
		return
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		log.Errorf("removed %s but could not open its directory to sync: %v", path, err)
		return
	}
	if err = dir.Sync(); err != nil {
		log.Errorf("removed %s but the sync failed: %v", path, err)
	}
	_ = dir.Close()
	log.Infof("%s %s consumed and removed", what, path)
}

// writeFile0600 creates the file with 0600 from the start and fixes the mode of
// one that already exists.
func writeFile0600(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func randomPassword(length int) (string, error) {
	limit := big.NewInt(int64(len(seedPasswordAlphabet)))
	password := make([]byte, length)
	for index := range password {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		password[index] = seedPasswordAlphabet[n.Int64()]
	}
	return string(password), nil
}

func newTokenVersion() (uint64, error) {
	var data [8]byte
	for {
		if _, err := rand.Read(data[:]); err != nil {
			return 0, err
		}
		version := binary.LittleEndian.Uint64(data[:])
		if version != 0 && version != ^uint64(0) {
			return version, nil
		}
	}
}
