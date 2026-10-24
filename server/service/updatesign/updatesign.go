// Package updatesign owns signature verification for everything the device
// installs from an update source: the OTA tarball and the picoclaw runtime.
//
// Every artifact must carry a razor-os updatesig envelope (served as
// "<artifact>.sig") that chains to the pinned firmware issuer CA embedded
// here. Verification is fail closed: any problem is an error, and callers
// install nothing on error.
package updatesign

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nzions/razor-os/linux/app/updatesig"
)

// MaxEnvelopeSize caps a downloaded .sig file.
const MaxEnvelopeSize = 16 * 1024

// DefaultBaseURL is the update source used when none is configured.
const DefaultBaseURL = "https://hc.nikspace.net/nanokvm"

// SigSuffix is appended to an artifact URL to name its envelope.
const SigSuffix = ".sig"

// clockFloor is the earliest time the device clock may read for a signature
// to be checked. The board boots at 1970 until NTP steps the clock, and a
// certificate validity check against a guessed time proves nothing.
var clockFloor = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

//go:embed firmware-ca.pem
var firmwareCAPEM []byte

//go:embed niks-root.pem
var niksRootPEM []byte

// Verifier checks update envelopes against one pinned issuer.
type Verifier struct {
	issuer    *x509.Certificate
	issuerErr error
	now       func() time.Time
}

// NewVerifier builds a verifier for the given issuer PEM and clock. A parse
// failure is remembered and makes every Verify fail.
func NewVerifier(issuerPEM []byte, now func() time.Time) *Verifier {
	issuer, err := updatesig.ParseIssuer(issuerPEM)
	return &Verifier{issuer: issuer, issuerErr: err, now: now}
}

var defaultVerifier = NewVerifier(firmwareCAPEM, time.Now)

// AppName is the signed version string of the OTA tarball for a release.
func AppName(version string) string {
	return "nanokvm-" + version + "-riscv64"
}

// PicoclawName is the signed version string of the picoclaw runtime.
func PicoclawName(version string) string {
	return "picoclaw-" + version + "-riscv64"
}

// Verify checks envelope as a KindApp signature over name and sha256 against
// the pinned firmware CA and returns the signer's common name.
func (v *Verifier) Verify(name string, envelope []byte, sum [sha256.Size]byte) (string, error) {
	if v == nil || v.issuerErr != nil {
		err := errors.New("no usable pinned issuer")
		if v != nil && v.issuerErr != nil {
			err = v.issuerErr
		}
		return "", fmt.Errorf("update signature cannot be checked: %w", err)
	}
	now := v.now()
	if now.Before(clockFloor) {
		return "", fmt.Errorf("device clock is not set (reads %s): refusing to check an update signature until the time is synchronised", now.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("update signature: empty signed name")
	}
	signer, err := updatesig.Verify(v.issuer, envelope, updatesig.KindApp, name, sum, now)
	if err != nil {
		return "", fmt.Errorf("update signature refused for %s: %w", name, err)
	}
	return signer, nil
}

// Verify checks envelope against the embedded pinned issuer.
func Verify(name string, envelope []byte, sum [sha256.Size]byte) (string, error) {
	return defaultVerifier.Verify(name, envelope, sum)
}

// RequireHTTPS refuses any URL that is not https.
func RequireHTTPS(raw string) error {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "https://") {
		return errors.New("update server URL must use https")
	}
	return nil
}

// TLSConfig trusts the system pool plus Nik's root CA.
func TLSConfig() *tls.Config {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(niksRootPEM)
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// Transport returns an HTTP transport using TLSConfig.
func Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = TLSConfig()
	return t
}

// ReadEnvelope reads at most MaxEnvelopeSize bytes and fails if there is more.
func ReadEnvelope(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxEnvelopeSize+1))
	if err != nil {
		return nil, fmt.Errorf("read update signature: %w", err)
	}
	if len(data) > MaxEnvelopeSize {
		return nil, fmt.Errorf("update signature exceeds %d bytes", MaxEnvelopeSize)
	}
	if len(data) == 0 {
		return nil, errors.New("update signature is empty")
	}
	return data, nil
}

// FetchEnvelope downloads "<artifactURL>.sig" with client. artifactURL must be
// https. Credentials in the URL (user:pass@) are sent as basic auth.
func FetchEnvelope(client *http.Client, artifactURL string) ([]byte, error) {
	if err := RequireHTTPS(artifactURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, artifactURL+SigSuffix, nil)
	if err != nil {
		return nil, fmt.Errorf("update signature request: %w", err)
	}
	if req.URL.User != nil {
		pw, _ := req.URL.User.Password()
		req.SetBasicAuth(req.URL.User.Username(), pw)
		req.URL.User = nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("update signature is inaccessible")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update signature unavailable: status %d", resp.StatusCode)
	}
	return ReadEnvelope(resp.Body)
}
