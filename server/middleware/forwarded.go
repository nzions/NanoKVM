package middleware

import (
	"net"
	"net/http"
	"strings"

	"NanoKVM-Server/config"
)

// ForwardedProto returns the scheme a trusted reverse proxy reports in
// X-Forwarded-Proto, or "" when the header is absent or the immediate peer is
// not listed in security.trustedProxies. The peer is the TCP remote address:
// gin's ClientIP reads forwarding headers itself and must not decide this.
func ForwardedProto(req *http.Request) string {
	if req == nil {
		return ""
	}
	value := strings.TrimSpace(strings.Split(req.Header.Get("X-Forwarded-Proto"), ",")[0])
	if value == "" || !isTrustedProxy(req.RemoteAddr, config.GetInstance().Security.TrustedProxies) {
		return ""
	}
	return strings.ToLower(value)
}

// isTrustedProxy reports whether remoteAddr's IP is one of the entries, each a
// bare IP or a CIDR. Entries that do not parse are ignored.
func isTrustedProxy(remoteAddr string, trusted []string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}

	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if _, network, err := net.ParseCIDR(entry); err == nil {
			if network.Contains(ip) {
				return true
			}
			continue
		}
		if proxy := net.ParseIP(entry); proxy != nil && proxy.Equal(ip) {
			return true
		}
	}
	return false
}
