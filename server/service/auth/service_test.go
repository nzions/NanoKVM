package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"NanoKVM-Server/authn"
	"NanoKVM-Server/config"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/proto"
	"NanoKVM-Server/utils"

	"github.com/gin-gonic/gin"
)

const testSeedPassword = "seed-password"

// newTestStore returns a store whose root password is testSeedPassword in a
// temporary shadow file and is already changed (MustChangePassword cleared).
func newTestStore(t *testing.T) *authn.Store {
	t.Helper()
	original, originalRoot := utils.ShadowFile, utils.RootPassFile
	utils.ShadowFile, utils.RootPassFile = filepath.Join(t.TempDir(), "shadow"), filepath.Join(t.TempDir(), "root.pass")
	t.Cleanup(func() { utils.ShadowFile, utils.RootPassFile = original, originalRoot })
	if err := os.WriteFile(utils.ShadowFile, []byte("daemon:*:19000:0:99999:7:::\nroot:*:19000:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := authn.NewStore(filepath.Join(t.TempDir(), "pwd"))
	if _, err := store.SetPassword(testSeedPassword); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestLoginCookieLifecycle(t *testing.T) {
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

	cookie := loginCookie(t, router, "root", testSeedPassword)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("unsafe session cookie: %+v", cookie)
	}
	recorder := requestJSONRecorder(router, http.MethodGet, "/account", nil, cookie)
	if recorder.Code != http.StatusOK {
		t.Fatalf("account status = %d", recorder.Code)
	}
	var rsp struct {
		Data proto.GetAccountRsp `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Data.Username != "root" || rsp.Data.Role != "admin" {
		t.Fatalf("account = %+v, want root/admin", rsp.Data)
	}

	if code := requestJSON(router, http.MethodPost, "/logout", nil, cookie); code != http.StatusOK {
		t.Fatalf("logout status = %d", code)
	}
	if code := requestJSON(router, http.MethodGet, "/account", nil, cookie); code != http.StatusUnauthorized {
		t.Fatalf("logged out session status = %d, want 401", code)
	}
}

func TestOnlyRootCanLogIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)

	for _, tc := range []struct{ user, password string }{
		{"admin", testSeedPassword}, // the retired account, even with the right password
		{"root", "wrong-password"},
	} {
		recorder := requestJSONRecorder(router, http.MethodPost, "/login",
			map[string]any{"username": tc.user, "password": tc.password}, nil)
		var rsp proto.Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &rsp); err != nil {
			t.Fatal(err)
		}
		if rsp.Code == 0 {
			t.Fatalf("login as %q with %q succeeded", tc.user, tc.password)
		}
	}
}

func TestSelfPasswordChangeRequiresCurrentPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	authenticated := router.Group("/").Use(middleware.CheckToken())
	authenticated.POST("/password", service.ChangePassword)
	cookie := loginCookie(t, router, "root", testSeedPassword)

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
	if ok, err := utils.VerifyRootPassword("new-password"); err != nil || ok {
		t.Fatalf("password changed without current credential: ok=%v err=%v", ok, err)
	}

	requestJSON(router, http.MethodPost, "/password", map[string]any{
		"currentPassword": testSeedPassword,
		"password":        "new-password",
	}, cookie)
	// the web password IS root's shadow password: one store
	if ok, err := utils.VerifyRootPassword("new-password"); err != nil || !ok {
		t.Fatalf("valid password change did not reach /etc/shadow: ok=%v err=%v", ok, err)
	}
	if ok, err := utils.VerifyRootPassword(testSeedPassword); err != nil || ok {
		t.Fatalf("old password still works: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Authenticate("root", "new-password"); err != nil || !ok {
		t.Fatalf("login with the new password: ok=%v err=%v", ok, err)
	}
	// the change revoked the session that made it
	if code := requestJSON(router, http.MethodPost, "/password", map[string]any{}, cookie); code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d, want 401", code)
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
	loginCookie(t, router, "root", testSeedPassword)

	// an empty password and a password sent through the retired
	// fixed-key encoding (not the stored one) are both refused
	for _, password := range []string{"", "U2FsdGVkX1+not-the-password"} {
		recorder := requestJSONRecorder(router, http.MethodPost, "/login",
			map[string]any{"username": "root", "password": password}, nil)
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

func TestLockoutBlocksEvenTheRightPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	conf := config.GetInstance()
	originalMax, originalDuration := conf.Security.LoginMaxFailures, conf.Security.LoginLockoutDuration
	conf.Security.LoginMaxFailures, conf.Security.LoginLockoutDuration = 2, 60
	defer func() {
		conf.Security.LoginMaxFailures, conf.Security.LoginLockoutDuration = originalMax, originalDuration
		ClearLoginAttempt("192.0.2.1")
	}()

	// httptest requests come from 192.0.2.1
	ClearLoginAttempt("192.0.2.1")
	RecordLoginFailure("192.0.2.1")
	RecordLoginFailure("192.0.2.1")

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	recorder := requestJSONRecorder(router, http.MethodPost, "/login",
		map[string]any{"username": "root", "password": testSeedPassword}, nil)
	var rsp proto.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Code != -5 {
		t.Fatalf("locked login code = %d, want -5", rsp.Code)
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == middleware.CookieName {
			t.Fatal("locked login set a session cookie")
		}
	}
}

// newUnsetStore is a first boot without /boot/root.pass: root has the stock
// password, the state does not exist yet.
func newUnsetStore(t *testing.T) *authn.Store {
	t.Helper()
	original, originalRoot := utils.ShadowFile, utils.RootPassFile
	utils.ShadowFile, utils.RootPassFile = filepath.Join(t.TempDir(), "shadow"), filepath.Join(t.TempDir(), "root.pass")
	t.Cleanup(func() { utils.ShadowFile, utils.RootPassFile = original, originalRoot })
	if err := os.WriteFile(utils.ShadowFile, []byte("daemon:*:19000:0:99999:7:::\nroot:"+utils.MD5Crypt("root", "stocksal")+":19000:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return authn.NewStore(filepath.Join(t.TempDir(), "pwd"))
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) proto.Response {
	t.Helper()
	var rsp proto.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	return rsp
}

func hasSessionCookie(recorder *httptest.ResponseRecorder) bool {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == middleware.CookieName && cookie.Value != "" {
			return true
		}
	}
	return false
}

func TestFirstConnectionSetupThenLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newUnsetStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/login", service.Login)
	router.GET("/state", service.GetState)
	router.POST("/setup", service.Setup)
	authenticated := router.Group("/").Use(middleware.CheckToken())
	authenticated.GET("/account", service.GetAccount)

	// the state says unset, with no session
	recorder := requestJSONRecorder(router, http.MethodGet, "/state", nil, nil)
	var state struct {
		Data proto.AuthStateRsp `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Data.PasswordSet {
		t.Fatalf("state = %+v, want password_set false", state.Data)
	}

	// login while unset: the distinct code, no cookie, not a counted failure
	recorder = requestJSONRecorder(router, http.MethodPost, "/login",
		map[string]any{"username": "root", "password": "root"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code != proto.ErrCodePasswordUnset || !strings.Contains(rsp.Msg, "/api/auth/setup") {
		t.Fatalf("login while unset = %+v, want code %d naming the setup endpoint", rsp, proto.ErrCodePasswordUnset)
	}
	if hasSessionCookie(recorder) {
		t.Fatal("login while unset set a session cookie")
	}

	// an invalid password is refused and leaves the state unset
	recorder = requestJSONRecorder(router, http.MethodPost, "/setup", map[string]any{"password": "a:b"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code == 0 || hasSessionCookie(recorder) {
		t.Fatalf("setup with an invalid password = %+v", rsp)
	}

	// setup logs in
	recorder = requestJSONRecorder(router, http.MethodPost, "/setup", map[string]any{"password": "chosen-password"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code != 0 {
		t.Fatalf("setup = %+v", rsp)
	}
	var cookie *http.Cookie
	for _, c := range recorder.Result().Cookies() {
		if c.Name == middleware.CookieName {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("setup session cookie: %+v", cookie)
	}
	if code := requestJSON(router, http.MethodGet, "/account", nil, cookie); code != http.StatusOK {
		t.Fatalf("account with the setup session = %d", code)
	}

	// a second setup is refused and sets no session
	recorder = requestJSONRecorder(router, http.MethodPost, "/setup", map[string]any{"password": "attacker-password"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code != proto.ErrCodeSetupDone || hasSessionCookie(recorder) {
		t.Fatalf("second setup = %+v, want code %d", rsp, proto.ErrCodeSetupDone)
	}

	// the state now says set, and login works with the chosen password only
	recorder = requestJSONRecorder(router, http.MethodGet, "/state", nil, nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Data.PasswordSet {
		t.Fatalf("state = %+v, want password_set true", state.Data)
	}
	loginCookie(t, router, "root", "chosen-password")
	recorder = requestJSONRecorder(router, http.MethodPost, "/login",
		map[string]any{"username": "root", "password": "attacker-password"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code == 0 {
		t.Fatal("login with the refused second setup's password succeeded")
	}
}

func TestRootPassStillWinsOverSetup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// a device with a password already set (what /boot/root.pass produces)
	store := newTestStore(t)
	restore := useTestStore(store)
	defer restore()

	service := NewService()
	router := gin.New()
	router.POST("/setup", service.Setup)
	router.GET("/state", service.GetState)

	recorder := requestJSONRecorder(router, http.MethodPost, "/setup", map[string]any{"password": "attacker-password"}, nil)
	if rsp := decodeResponse(t, recorder); rsp.Code != proto.ErrCodeSetupDone || hasSessionCookie(recorder) {
		t.Fatalf("setup on a set device = %+v", rsp)
	}
	if ok, err := utils.VerifyRootPassword(testSeedPassword); err != nil || !ok {
		t.Fatalf("setup changed the password: ok=%v err=%v", ok, err)
	}
}
