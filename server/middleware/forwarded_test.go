package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"NanoKVM-Server/config"
)

func TestIsTrustedProxy(t *testing.T) {
	trusted := []string{"10.0.0.0/8", "192.168.1.5", "::1", "not-an-ip"}
	cases := map[string]bool{
		"10.2.3.4:5000":     true,
		"192.168.1.5:80":    true,
		"192.168.1.6:80":    false,
		"[::1]:443":         true,
		"203.0.113.9:1234":  false,
		"10.2.3.4":          true,
		"garbage":           false,
		"":                  false,
		"not-an-ip:80":      false,
		"[2001:db8::1]:443": false,
	}
	for remote, want := range cases {
		if got := isTrustedProxy(remote, trusted); got != want {
			t.Errorf("isTrustedProxy(%q) = %v, want %v", remote, got, want)
		}
	}
	if isTrustedProxy("10.2.3.4:1", nil) {
		t.Error("an empty list must trust nobody")
	}
}

func TestForwardedProto(t *testing.T) {
	conf := config.GetInstance()
	original := conf.Security.TrustedProxies
	defer func() { conf.Security.TrustedProxies = original }()

	request := func(remote, header string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		if header != "" {
			req.Header.Set("X-Forwarded-Proto", header)
		}
		return req
	}

	conf.Security.TrustedProxies = nil
	if got := ForwardedProto(request("10.0.0.1:1", "https")); got != "" {
		t.Errorf("no trusted proxies: got %q, want empty", got)
	}

	conf.Security.TrustedProxies = []string{"10.0.0.0/24"}
	if got := ForwardedProto(request("10.0.0.1:1", "HTTPS, http")); got != "https" {
		t.Errorf("trusted peer: got %q, want https", got)
	}
	if got := ForwardedProto(request("198.51.100.7:1", "https")); got != "" {
		t.Errorf("untrusted peer: got %q, want empty", got)
	}
	if got := ForwardedProto(request("10.0.0.1:1", "")); got != "" {
		t.Errorf("no header: got %q, want empty", got)
	}
}
