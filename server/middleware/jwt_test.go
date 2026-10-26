package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"NanoKVM-Server/authn"
	"NanoKVM-Server/config"
	"NanoKVM-Server/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const testSeedPassword = "seed-password"

// useTestShadow points the shadow file at a temporary one holding a locked
// root entry.
func useTestShadow(t *testing.T) {
	t.Helper()
	original, originalRoot := utils.ShadowFile, utils.RootPassFile
	utils.ShadowFile, utils.RootPassFile = filepath.Join(t.TempDir(), "shadow"), filepath.Join(t.TempDir(), "root.pass")
	t.Cleanup(func() { utils.ShadowFile, utils.RootPassFile = original, originalRoot })
	if err := os.WriteFile(utils.ShadowFile, []byte("daemon:*:19000:0:99999:7:::\nroot:*:19000:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newTestStore returns a store whose root password is testSeedPassword and
// is already changed (MustChangePassword cleared).
func newTestStore(t *testing.T) *authn.Store {
	t.Helper()
	return newTestStoreAt(t, filepath.Join(t.TempDir(), "pwd"))
}

func newTestStoreAt(t *testing.T, path string) *authn.Store {
	t.Helper()
	useTestShadow(t)
	store := authn.NewStore(path)
	if _, err := store.SetPassword(testSeedPassword); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCheckTokenBlocksUntilPasswordChanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useTestShadow(t)
	path := filepath.Join(t.TempDir(), "pwd")
	store := authn.NewStore(path)
	seed, err := os.ReadFile(filepath.Join(filepath.Dir(path), "initial.pass"))
	if err == nil {
		t.Fatalf("seed file exists before first use: %q", seed)
	}
	// first use seeds root and writes the seed password
	if err = store.Init(); err != nil {
		t.Fatal(err)
	}
	seed, err = os.ReadFile(filepath.Join(filepath.Dir(path), "initial.pass"))
	if err != nil {
		t.Fatal(err)
	}
	owner, ok, err := store.Authenticate("root", strings.TrimSpace(string(seed)))
	if err != nil || !ok {
		t.Fatalf("seed login: ok=%v err=%v", ok, err)
	}
	restore := useTestAuthStore(t, store)
	defer restore()
	token, err := GenerateJWT(owner.Username, owner.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	ok200 := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router.GET("/api/auth/password", CheckToken(), ok200)
	router.POST("/api/auth/logout", CheckToken(), ok200)
	router.GET("/api/vm/info", CheckToken(), ok200)
	if status := requestWithToken(router, "/api/auth/password", token); status != http.StatusNoContent {
		t.Fatalf("password check status = %d", status)
	}
	if status := requestWithToken(router, "/api/vm/info", token); status != http.StatusForbidden {
		t.Fatalf("other route status = %d, want 403", status)
	}

	updated, err := store.SetPassword("a-new-password")
	if err != nil {
		t.Fatal(err)
	}
	token, err = GenerateJWT(updated.Username, updated.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	if status := requestWithToken(router, "/api/vm/info", token); status != http.StatusNoContent {
		t.Fatalf("after change status = %d", status)
	}
}

func TestCheckTokenUsesLiveRoleAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	admin, ok, err := store.Authenticate("root", testSeedPassword)
	if err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}

	restore := useTestAuthStore(t, store)
	defer restore()
	adminToken, err := GenerateJWT(admin.Username, admin.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	// a token for the retired admin account names no user any more
	userToken, err := GenerateJWT("admin", admin.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/admin", CheckToken(), RequireRole(authn.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	if status := requestWithToken(router, "/admin", ""); status != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", status)
	}
	if status := requestWithToken(router, "/admin", adminToken); status != http.StatusNoContent {
		t.Fatalf("admin status = %d", status)
	}
	if status := requestWithToken(router, "/admin", userToken); status != http.StatusUnauthorized {
		t.Fatalf("retired admin token status = %d, want 401", status)
	}
	if _, err = store.Revoke("root"); err != nil {
		t.Fatal(err)
	}
	if status := requestWithToken(router, "/admin", adminToken); status != http.StatusUnauthorized {
		t.Fatalf("revoked status = %d", status)
	}
}

func TestParseJWTRejectsOtherHMACMethods(t *testing.T) {
	conf := config.GetInstance()
	originalSecret := conf.JWT.SecretKey
	conf.JWT.SecretKey = "test-secret"
	defer func() { conf.JWT.SecretKey = originalSecret }()

	now := time.Now()
	claims := Token{
		Username:     "admin",
		TokenVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(conf.JWT.SecretKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseJWT(token); err == nil {
		t.Fatal("HS384 token was accepted")
	}
}

func TestParseJWTRejectsExpiredToken(t *testing.T) {
	conf := config.GetInstance()
	originalSecret := conf.JWT.SecretKey
	conf.JWT.SecretKey = "test-secret"
	defer func() { conf.JWT.SecretKey = originalSecret }()

	claims := Token{
		Username:     "admin",
		TokenVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(conf.JWT.SecretKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseJWT(token); err == nil {
		t.Fatal("expired token was accepted")
	}
}

func TestRevokeUserSessionsCancelsActiveRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unregister := activeSessions.register("alice", cancel)
	defer unregister()

	RevokeUserSessions("alice")
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("active session was not cancelled")
	}
}

func TestAccountFileResetCancelsActiveSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwd")
	store := newTestStoreAt(t, path)
	user, ok, err := store.Authenticate("root", testSeedPassword)
	if err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	restore := useTestAuthStore(t, store)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchSessionState(ctx, cancel, user.Username, user.TokenVersion, 10*time.Millisecond)
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("active session survived account-file reset")
	}
}

func TestWatchWebSocketClosesRevokedConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		stop := WatchWebSocket(ctx, connection)
		defer stop()
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	cancel()
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = connection.ReadMessage()
	if !websocket.IsCloseError(err, SessionRevokedCloseCode) {
		t.Fatalf("close error = %v, want code %d", err, SessionRevokedCloseCode)
	}
}

func TestCheckWebSocketOrigin(t *testing.T) {
	tests := []struct {
		origin string
		host   string
		want   bool
	}{
		{"", "kvm.local", true},
		{"http://kvm.local", "kvm.local", true},
		{"http://KVM.local", "kvm.local", true},
		{"http://kvm.local:8080", "kvm.local:8080", true},
		{"http://kvm.local:8081", "kvm.local:8080", false},
		{"http://evil.example", "kvm.local", false},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "http://"+test.host+"/api/ws", nil)
		request.Host = test.host
		if test.origin != "" {
			request.Header.Set("Origin", test.origin)
		}
		if got := CheckWebSocketOrigin(request); got != test.want {
			t.Fatalf("origin %q host %q = %v, want %v", test.origin, test.host, got, test.want)
		}
	}
}

func requestWithToken(handler http.Handler, path, token string) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func useTestAuthStore(t *testing.T, store *authn.Store) func() {
	t.Helper()
	originalStore := authn.DefaultStore
	conf := config.GetInstance()
	originalAuthentication := conf.Authentication
	originalSecret := conf.JWT.SecretKey
	originalDuration := conf.JWT.RefreshTokenDuration
	authn.DefaultStore = store
	conf.Authentication = "enable"
	conf.JWT.SecretKey = "test-secret"
	conf.JWT.RefreshTokenDuration = 3600
	return func() {
		authn.DefaultStore = originalStore
		conf.Authentication = originalAuthentication
		conf.JWT.SecretKey = originalSecret
		conf.JWT.RefreshTokenDuration = originalDuration
	}
}
