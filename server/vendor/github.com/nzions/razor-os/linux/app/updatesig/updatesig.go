// Package updatesig is the one definition of what a razor update signature
// covers and how it is carried (docs/plans/ab-slots.md §3, §6). The board
// verifies with it and the publish tool signs with it; neither carries its
// own copy.
//
// TRUST. The signer is a leaf certificate issued from Nik's CA (sage `ca …
// issue signer`: EKU codeSigning) by the `firmware` issuer. The board pins
// that issuer's certificate — never a signer key — so rotating the signer is
// issuing a new leaf, with nothing on the board changing (Nik, 2026-09-23:
// "can it be issued from our ca?").
//
// WHAT IS SIGNED. A 32-byte digest binding the kind, the version and the
// artifact's own sha256:
//
//	sha256("razor-update\x00" + kind + "\x00" + version + "\x00" + sha256(artifact))
//
// so a kernel signature never installs as an app and a signed artifact cannot
// be relabelled with another version. Signing the artifact's digest rather
// than the artifact lets the board stream a download to disk while hashing it.
// The signer signs that digest with its certificate's key and algorithm
// (for ECDSA, x509.CheckSignature hashes it once more with SHA-256).
//
// THE ENVELOPE. One PEM file: a "RAZOR UPDATE SIGNATURE" block (the raw
// signature) then the signer's "CERTIFICATE" block. It is the `<artifact>.sig`
// hatch serves, and, base64-encoded, the X-Razor-Sig header of a pushed update.
//
// ONE OWNER, TWO CONSUMERS. This is its own module so that both the razor
// board (linux/app, which takes it from this tree) and the pid1 appliance
// (linux/pid1 verifies a release; unity's forge signs one) require this
// definition rather than carrying a copy (ruling 2026-10-13: appliance
// releases converge onto this envelope and Nik's CA `firmware` issuer).
package updatesig

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kinds a signature may cover. Anything else is refused by Message.
// KindApp, KindConfig and KindKernel are a razor board's updates; KindRelease
// is a pid1 appliance release, the artifact being its OCI image manifest and
// the version its release name.
const (
	KindApp     = "app"
	KindConfig  = "config"
	KindKernel  = "kernel"
	KindRelease = "release"
)

const (
	domain       = "razor-update"
	sigBlockType = "RAZOR UPDATE SIGNATURE"
	maxEnvelope  = 16 << 10 // a signature and one certificate; anything bigger is not ours
)

// ErrKind is returned for a kind outside KindApp/KindConfig/KindKernel/KindRelease.
var ErrKind = errors.New("updatesig: unknown update kind")

// ErrVersion is returned for an empty version or one carrying a NUL, which
// would make the message ambiguous.
var ErrVersion = errors.New("updatesig: version is empty or contains NUL")

// Message returns the 32-byte digest an update signature covers.
func Message(kind, version string, artifact [sha256.Size]byte) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	switch kind {
	case KindApp, KindConfig, KindKernel, KindRelease:
	default:
		return zero, ErrKind
	}
	if version == "" || strings.IndexByte(version, 0) >= 0 {
		return zero, ErrVersion
	}
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(version))
	h.Write([]byte{0})
	h.Write(artifact[:])
	var m [sha256.Size]byte
	copy(m[:], h.Sum(nil))
	return m, nil
}

// signatureAlgorithm is the x509 algorithm a signer's key signs with here:
// ECDSA with SHA-256 (what sage mints) or Ed25519. Anything else is refused.
func signatureAlgorithm(pk x509.PublicKeyAlgorithm) (x509.SignatureAlgorithm, crypto.Hash, error) {
	switch pk {
	case x509.ECDSA:
		return x509.ECDSAWithSHA256, crypto.SHA256, nil
	case x509.Ed25519:
		return x509.PureEd25519, 0, nil
	}
	return 0, 0, fmt.Errorf("updatesig: signer key type %v is not ECDSA or Ed25519", pk)
}

// Sign signs an update with the signer's key and returns the envelope. certPEM
// is the signer's certificate (one CERTIFICATE block); key must be its private
// key.
func Sign(key crypto.Signer, certPEM []byte, kind, version string, artifact [sha256.Size]byte) ([]byte, error) {
	cert, err := parseCert(certPEM)
	if err != nil {
		return nil, err
	}
	if !publicKeyEqual(cert.PublicKey, key.Public()) {
		return nil, errors.New("updatesig: the key is not the certificate's key")
	}
	_, hash, err := signatureAlgorithm(cert.PublicKeyAlgorithm)
	if err != nil {
		return nil, err
	}
	m, err := Message(kind, version, artifact)
	if err != nil {
		return nil, err
	}
	signed := m[:]
	if hash != 0 {
		d := sha256.Sum256(m[:])
		signed = d[:]
	}
	sig, err := key.Sign(rand.Reader, signed, hash)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := pem.Encode(&out, &pem.Block{Type: sigBlockType, Bytes: sig}); err != nil {
		return nil, err
	}
	if err := pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Verify checks an envelope against the pinned issuer at time now, and
// returns the signer's common name. Fail-closed: any parse error, a chain
// that does not reach issuer, a certificate without the codeSigning EKU or
// outside its validity at now, or a bad signature is an error.
//
// now must be a real wall-clock time. This board boots at 1970 until NTP
// steps the clock; the caller refuses updates until then rather than passing
// a guessed time.
func Verify(issuer *x509.Certificate, envelope []byte, kind, version string, artifact [sha256.Size]byte, now time.Time) (string, error) {
	if issuer == nil {
		return "", errors.New("updatesig: no pinned issuer")
	}
	if len(envelope) > maxEnvelope {
		return "", errors.New("updatesig: envelope too large")
	}
	sigBlock, rest := pem.Decode(envelope)
	if sigBlock == nil || sigBlock.Type != sigBlockType {
		return "", errors.New("updatesig: envelope does not start with a signature block")
	}
	certBlock, rest := pem.Decode(rest)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return "", errors.New("updatesig: envelope carries no signer certificate")
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return "", errors.New("updatesig: trailing data after the signer certificate")
	}
	leaf, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return "", err
	}
	roots := x509.NewCertPool()
	roots.AddCert(issuer)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}); err != nil {
		return "", fmt.Errorf("updatesig: signer certificate: %w", err)
	}
	alg, _, err := signatureAlgorithm(leaf.PublicKeyAlgorithm)
	if err != nil {
		return "", err
	}
	m, err := Message(kind, version, artifact)
	if err != nil {
		return "", err
	}
	if err := leaf.CheckSignature(alg, m[:], sigBlock.Bytes); err != nil {
		return "", fmt.Errorf("updatesig: signature: %w", err)
	}
	return leaf.Subject.CommonName, nil
}

// ParseIssuer reads the pinned issuer certificate (one PEM CERTIFICATE block,
// /etc/razor-update-ca.pem on the board). It must be a CA.
func ParseIssuer(certPEM []byte) (*x509.Certificate, error) {
	c, err := parseCert(certPEM)
	if err != nil {
		return nil, err
	}
	if !c.IsCA {
		return nil, errors.New("updatesig: the pinned certificate is not a CA")
	}
	return c, nil
}

func parseCert(certPEM []byte) (*x509.Certificate, error) {
	b, _ := pem.Decode(certPEM)
	if b == nil || b.Type != "CERTIFICATE" {
		return nil, errors.New("updatesig: not a PEM CERTIFICATE block")
	}
	return x509.ParseCertificate(b.Bytes)
}

func publicKeyEqual(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	e, ok := a.(equaler)
	return ok && e.Equal(b)
}
