package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"NanoKVM-Server/config"

	"github.com/gin-gonic/gin"
)

// F6: the picoclaw internal token reaches HID (and the screenshot that feeds
// it) and nothing else.
func TestLoopbackInternalTokenPermits(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/api/picoclaw/screenshot"},
		{http.MethodPost, "/api/picoclaw/actions"},
		{http.MethodPost, "/api/picoclaw/mcp"},
		{http.MethodGet, "/api/picoclaw/runtime/session"},
		{http.MethodPost, "/api/internal/usb/recover"},
	}
	for _, c := range allowed {
		if !LoopbackInternalTokenPermits(c.method, c.path) {
			t.Errorf("%s %s refused, want allowed", c.method, c.path)
		}
	}

	refused := []struct{ method, path string }{
		{http.MethodPost, "/api/picoclaw/load-image"},
		{http.MethodPost, "/api/picoclaw/model/config"},
		{http.MethodPost, "/api/picoclaw/agent/profile"},
		{http.MethodPost, "/api/picoclaw/runtime/install"},
		{http.MethodPost, "/api/picoclaw/runtime/uninstall"},
		{http.MethodPost, "/api/picoclaw/runtime/start"},
		{http.MethodDelete, "/api/picoclaw/runtime/session"},
		{http.MethodGet, "/api/picoclaw/sessions"},
		{http.MethodGet, "/api/picoclaw/gateway/ws"},
		{http.MethodGet, "/api/mcp/config"},
		{http.MethodPost, "/api/mcp/key/regenerate"},
		{http.MethodPost, "/api/hid/mode"},
		{http.MethodPost, "/api/application/update"},
		{http.MethodPost, "/api/vm/script/upload"},
		{http.MethodGet, "/api/picoclaw/actions"},
		{http.MethodPost, "/api/picoclaw/screenshot"},
		{http.MethodPost, "/api/picoclaw/actions/"},
	}
	for _, c := range refused {
		if LoopbackInternalTokenPermits(c.method, c.path) {
			t.Errorf("%s %s allowed, want refused", c.method, c.path)
		}
	}
}

func TestCheckLoopbackInternalTokenAllowsOnlyHIDRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token, err := config.GetPicoclawInternalToken()
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	guard := CheckLoopbackInternalToken()
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	r.POST("/api/picoclaw/actions", guard, ok)
	r.POST("/api/picoclaw/load-image", guard, ok)
	r.POST("/api/picoclaw/runtime/start", guard, ok)

	do := func(path string, withToken string, remote string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = remote
		if withToken != "" {
			req.Header.Set(config.PicoclawInternalTokenHeader, withToken)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	const loopback = "127.0.0.1:40000"
	if got := do("/api/picoclaw/actions", token, loopback); got != http.StatusOK {
		t.Errorf("HID actions with token = %d, want 200", got)
	}
	for _, path := range []string{"/api/picoclaw/load-image", "/api/picoclaw/runtime/start"} {
		if got := do(path, token, loopback); got != http.StatusForbidden {
			t.Errorf("%s with valid token = %d, want 403", path, got)
		}
	}
	if got := do("/api/picoclaw/actions", "wrong", loopback); got != http.StatusUnauthorized {
		t.Errorf("bad token = %d, want 401", got)
	}
	if got := do("/api/picoclaw/actions", token, "192.0.2.10:40000"); got != http.StatusUnauthorized {
		t.Errorf("non-loopback with token = %d, want 401", got)
	}
}
