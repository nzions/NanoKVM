package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"NanoKVM-Server/authn"
	"NanoKVM-Server/config"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
)

const testSeedPassword = "seed-password"

// newTestStore returns a store whose owner password is testSeedPassword and
// is already changed (MustChangePassword cleared).
func newTestStore(t *testing.T) *authn.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pwd")
	seedPath := filepath.Join(filepath.Dir(path), "initial.pass")
	if err := os.WriteFile(seedPath, []byte(testSeedPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := authn.NewStore(path)
	if _, err := store.SetPassword("admin", testSeedPassword); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestLoginCookieAndUserAuthorizationLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	authenticated := router.Group("/").Use(middleware.CheckToken())
	authenticated.GET("/account", service.GetAccount)
	authenticated.POST("/logout", service.Logout)
	admin := router.Group("/").Use(middleware.CheckToken(), middleware.RequireRole(authn.RoleAdmin))
	admin.GET("/users", service.ListUsers)
	admin.POST("/users", service.CreateUser)
	admin.PUT("/users/:username", service.UpdateUser)

	adminCookie := loginCookie(t, router, "admin", testSeedPassword)
	if !adminCookie.HttpOnly || adminCookie.SameSite != http.SameSiteStrictMode || adminCookie.Path != "/" {
		t.Fatalf("unsafe session cookie: %+v", adminCookie)
	}

	createBody := map[string]any{
		"username": "alice",
		"password": "valid-password",
		"role":     "user",
	}
	if code := requestJSON(router, http.MethodPost, "/users", createBody, adminCookie); code != http.StatusOK {
		t.Fatalf("create status = %d", code)
	}
	userCookie := loginCookie(t, router, "alice", "valid-password")
	if code := requestJSON(router, http.MethodGet, "/users", nil, userCookie); code != http.StatusForbidden {
		t.Fatalf("user management status = %d, want 403", code)
	}

	disabled := false
	if code := requestJSON(router, http.MethodPut, "/users/alice", map[string]any{"enabled": disabled}, adminCookie); code != http.StatusOK {
		t.Fatalf("disable status = %d", code)
	}
	if code := requestJSON(router, http.MethodGet, "/account", nil, userCookie); code != http.StatusUnauthorized {
		t.Fatalf("disabled session status = %d, want 401", code)
	}
}

func TestLogoutDoesNotInvalidateAnotherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err := store.Create("alice", "alice-password", authn.RoleUser); err != nil {
		t.Fatal(err)
	}
	if err := store.Create("bob", "bob-password", authn.RoleUser); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	authenticated := router.Group("/").Use(middleware.CheckToken())
	authenticated.GET("/account", service.GetAccount)
	authenticated.POST("/logout", service.Logout)

	aliceCookie := loginCookie(t, router, "alice", "alice-password")
	bobCookie := loginCookie(t, router, "bob", "bob-password")
	if code := requestJSON(router, http.MethodPost, "/logout", nil, aliceCookie); code != http.StatusOK {
		t.Fatalf("logout status = %d", code)
	}
	if code := requestJSON(router, http.MethodGet, "/account", nil, aliceCookie); code != http.StatusUnauthorized {
		t.Fatalf("logged out session status = %d", code)
	}
	if code := requestJSON(router, http.MethodGet, "/account", nil, bobCookie); code != http.StatusOK {
		t.Fatalf("other user's session status = %d", code)
	}
}

func TestNormalUserPasswordNeverChangesSystemPassword(t *testing.T) {
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err := store.Create("alice", "alice-password", authn.RoleUser); err != nil {
		t.Fatal(err)
	}

	originalUpdater := systemPasswordUpdater
	updates := 0
	systemPasswordUpdater = func(string) error {
		updates++
		return nil
	}
	defer func() { systemPasswordUpdater = originalUpdater }()

	if err := changeUserPassword("alice", "new-password"); err != nil {
		t.Fatal(err)
	}
	if updates != 0 {
		t.Fatalf("system password updated %d times for a normal user", updates)
	}
	if _, ok, err := store.Authenticate("alice", "new-password"); err != nil || !ok {
		t.Fatalf("new web password login: ok=%v err=%v", ok, err)
	}
}

func TestSelfPasswordChangeRequiresCurrentPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err := store.Create("alice", "alice-password", authn.RoleUser); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	authenticated := router.Group("/").Use(middleware.CheckToken())
	authenticated.POST("/password", service.ChangePassword)
	cookie := loginCookie(t, router, "alice", "alice-password")

	recorder := requestJSONRecorder(router, http.MethodPost, "/password", map[string]any{
		"currentPassword": "wrong-password",
		"password":        "new-password",
	}, cookie)
	var response proto.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode wrong-current-password response: %v", err)
	}
	if response.Code != -3 || response.Msg != "current password is incorrect" {
		t.Fatalf("wrong-current-password response = %+v", response)
	}
	if _, ok, err := store.Authenticate("alice", "new-password"); err != nil || ok {
		t.Fatalf("password changed without current credential: ok=%v err=%v", ok, err)
	}

	requestJSON(router, http.MethodPost, "/password", map[string]any{
		"currentPassword": "alice-password",
		"password":        "new-password",
	}, cookie)
	if _, ok, err := store.Authenticate("alice", "new-password"); err != nil || !ok {
		t.Fatalf("valid password change failed: ok=%v err=%v", ok, err)
	}
}

func TestAdministratorCannotResetDeviceOwnerPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}
	if err := store.Create("second-admin", "second-password", authn.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	originalUpdater := systemPasswordUpdater
	updates := 0
	systemPasswordUpdater = func(string) error {
		updates++
		return nil
	}
	defer func() { systemPasswordUpdater = originalUpdater }()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	admin := router.Group("/").Use(middleware.CheckToken(), middleware.RequireRole(authn.RoleAdmin))
	admin.POST("/users/:username/password", service.ChangeUserPassword)
	cookie := loginCookie(t, router, "second-admin", "second-password")
	requestJSON(router, http.MethodPost, "/users/admin/password", map[string]any{
		"password": "new-password",
	}, cookie)

	if updates != 0 {
		t.Fatalf("administrator changed the device owner's system password %d times", updates)
	}
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("device owner password changed: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Authenticate("admin", "new-password"); err != nil || ok {
		t.Fatalf("unauthorized password became active: ok=%v err=%v", ok, err)
	}
}

func TestSystemPasswordFailureRollsBackWebPassword(t *testing.T) {
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("default login: ok=%v err=%v", ok, err)
	}

	originalUpdater := systemPasswordUpdater
	systemPasswordUpdater = func(string) error { return errors.New("passwd failed") }
	defer func() { systemPasswordUpdater = originalUpdater }()

	if err := changeUserPassword("admin", "new-password"); err == nil {
		t.Fatal("system password failure was ignored")
	}
	if _, ok, err := store.Authenticate("admin", testSeedPassword); err != nil || !ok {
		t.Fatalf("old web password was not restored: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Authenticate("admin", "new-password"); err != nil || ok {
		t.Fatalf("failed password stayed active: ok=%v err=%v", ok, err)
	}
}

func loginCookie(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	body := map[string]any{"username": username, "password": password}
	recorder := httptest.NewRecorder()
	request := jsonRequest(http.MethodPost, "/login", body)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("token")) {
		t.Fatalf("login exposed JWT in response: %s", recorder.Body.String())
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == middleware.CookieName {
			return cookie
		}
	}
	t.Fatal("login did not set session cookie")
	return nil
}

func requestJSON(handler http.Handler, method, path string, body any, cookie *http.Cookie) int {
	return requestJSONRecorder(handler, method, path, body, cookie).Code
}

func requestJSONRecorder(handler http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := jsonRequest(method, path, body)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func jsonRequest(method, path string, body any) *http.Request {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func useTestStore(store *authn.Store) func() {
	originalStore := authn.DefaultStore
	conf := config.GetInstance()
	originalAuthentication := conf.Authentication
	originalSecret := conf.JWT.SecretKey
	originalDuration := conf.JWT.RefreshTokenDuration
	originalRevokeOnLogout := conf.JWT.RevokeTokensOnLogout
	authn.DefaultStore = store
	conf.Authentication = "enable"
	conf.JWT.SecretKey = "test-secret"
	conf.JWT.RefreshTokenDuration = 3600
	conf.JWT.RevokeTokensOnLogout = true
	return func() {
		authn.DefaultStore = originalStore
		conf.Authentication = originalAuthentication
		conf.JWT.SecretKey = originalSecret
		conf.JWT.RefreshTokenDuration = originalDuration
		conf.JWT.RevokeTokensOnLogout = originalRevokeOnLogout
	}
}

func TestLoginTakesPlainPasswordOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)

	// the plain password logs in
	loginCookie(t, router, "admin", testSeedPassword)

	// an empty password and a password sent through the retired
	// fixed-key encoding (not the stored one) are both refused
	for _, password := range []string{"", "U2FsdGVkX1+not-the-password"} {
		recorder := requestJSONRecorder(router, http.MethodPost, "/login",
			map[string]any{"username": "admin", "password": password}, nil)
		var rsp proto.Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &rsp); err != nil {
			t.Fatal(err)
		}
		if rsp.Code == 0 {
			t.Fatalf("login with password %q succeeded", password)
		}
		for _, cookie := range recorder.Result().Cookies() {
			if cookie.Name == middleware.CookieName {
				t.Fatalf("login with password %q set a session cookie", password)
			}
		}
	}
}
