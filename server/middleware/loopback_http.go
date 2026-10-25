package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"time"

	"NanoKVM-Server/config"
)

func ListenAndServeLoopbackHTTPRedirect(
	httpAddr string,
	httpsPort string,
	handler http.Handler,
	allowedPaths ...string,
) error {
	allowlist := make(map[string]struct{}, len(allowedPaths))
	for _, path := range allowedPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		allowlist[path] = struct{}{}
	}

	return NewServer(httpAddr, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isLoopbackAllowedPath(req, allowlist) {
			if hasValidLoopbackHTTPToken(req) {
				handler.ServeHTTP(w, req)
				return
			}

			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		http.Redirect(w, req, "https://"+redirectHost(req.Host, httpsPort)+req.URL.RequestURI(), http.StatusTemporaryRedirect)
	})).ListenAndServe()
}

func redirectHost(requestHost string, httpsPort string) string {
	host := requestHost
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}

	if httpsPort != "443" {
		return net.JoinHostPort(host, httpsPort)
	}

	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") && !strings.HasSuffix(host, "]") {
		return "[" + host + "]"
	}

	return host
}

// loopbackInternalTokenAllowlist is the complete set of (method, path) pairs
// the picoclaw internal token may reach. The token sits in picoclaw's own
// config, so it is held by an LLM agent: it may drive the remote host's HID
// (keyboard/mouse), look at the HDMI frame it is driving, read its own session
// and ask the HID gadget to recover. Nothing else - no file read, no image
// load, no management route. Any other request carrying a valid token is
// refused 403. This is the control; the prompts and picoclaw's tool config are
// only advice.
var loopbackInternalTokenAllowlist = map[string]struct{}{
	http.MethodGet + " /api/picoclaw/screenshot":      {}, // HDMI frame, the feedback for HID
	http.MethodPost + " /api/picoclaw/actions":        {}, // HID: click/move/drag/scroll/type/hotkey
	http.MethodPost + " /api/picoclaw/mcp":            {}, // kvm_screenshot + kvm_actions only
	http.MethodGet + " /api/picoclaw/runtime/session": {}, // its own session id
	http.MethodPost + " /api/internal/usb/recover":    {}, // HID gadget recovery
}

// LoopbackInternalTokenPermits reports whether the internal token may call
// method+path at all.
func LoopbackInternalTokenPermits(method string, path string) bool {
	_, ok := loopbackInternalTokenAllowlist[method+" "+path]
	return ok
}

// loopbackInternalTokenValid: a loopback caller with the right token. It says
// nothing about WHAT the caller may do; see LoopbackInternalTokenPermits.
func loopbackInternalTokenValid(req *http.Request) bool {
	return req != nil && isLoopbackRemote(req.RemoteAddr) && hasValidLoopbackHTTPToken(req)
}

func allowByLoopbackInternalToken(req *http.Request) bool {
	return loopbackInternalTokenValid(req) &&
		req.URL != nil &&
		LoopbackInternalTokenPermits(req.Method, req.URL.Path)
}

func isLoopbackAllowedPath(req *http.Request, allowedPaths map[string]struct{}) bool {
	if !allowByLoopbackInternalToken(req) {
		return false
	}

	_, allowed := allowedPaths[req.URL.Path]
	return allowed
}

func hasValidLoopbackHTTPToken(req *http.Request) bool {
	if req == nil {
		return false
	}
	token, err := config.GetPicoclawInternalToken()
	if err != nil || token == "" {
		return false
	}

	provided := req.Header.Get(config.PicoclawInternalTokenHeader)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func isLoopbackRemote(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// NewServer returns an http.Server with the timeouts every listener shares.
// There is deliberately no ReadTimeout or WriteTimeout: they cover the whole
// request and response and would cut the long-lived video streams and
// websockets. ReadHeaderTimeout stops slowloris and IdleTimeout reaps idle
// keep-alive connections.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
