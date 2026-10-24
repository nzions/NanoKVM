package updatesign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/nzions/razor-os/linux/app/updatesig"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newCA(t *testing.T, cn string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// leaf issues a codeSigning leaf and returns its PEM and key.
func (ca *testCA) leaf(t *testing.T) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-signer"},
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}

func sign(t *testing.T, ca *testCA, name string, sum [32]byte) []byte {
	t.Helper()
	leafPEM, key := ca.leaf(t)
	env, err := updatesig.Sign(key, leafPEM, updatesig.KindApp, name, sum)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func fixedClock(ts time.Time) func() time.Time { return func() time.Time { return ts } }

var goodNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func TestVerify(t *testing.T) {
	ca := newCA(t, "test-firmware")
	sum := sha256.Sum256([]byte("artifact"))
	name := AppName("1.2.3")
	env := sign(t, ca, name, sum)
	v := NewVerifier(ca.pem, fixedClock(goodNow))

	t.Run("valid", func(t *testing.T) {
		signer, err := v.Verify(name, env, sum)
		if err != nil {
			t.Fatalf("valid signature refused: %v", err)
		}
		if signer != "test-signer" {
			t.Fatalf("signer = %q", signer)
		}
	})

	t.Run("wrong name", func(t *testing.T) {
		if _, err := v.Verify(AppName("1.2.4"), env, sum); err == nil {
			t.Fatal("signature for another version accepted")
		}
		if _, err := v.Verify(PicoclawName("1.2.3"), env, sum); err == nil {
			t.Fatal("signature for another artifact name accepted")
		}
	})

	t.Run("tampered sha", func(t *testing.T) {
		bad := sha256.Sum256([]byte("other"))
		if _, err := v.Verify(name, env, bad); err == nil {
			t.Fatal("tampered artifact digest accepted")
		}
	})

	t.Run("other CA", func(t *testing.T) {
		other := newCA(t, "evil")
		evil := sign(t, other, name, sum)
		if _, err := v.Verify(name, evil, sum); err == nil {
			t.Fatal("leaf from another CA accepted")
		}
	})

	t.Run("clock before floor", func(t *testing.T) {
		early := NewVerifier(ca.pem, fixedClock(time.Unix(0, 0)))
		_, err := early.Verify(name, env, sum)
		if err == nil {
			t.Fatal("signature checked with an unset clock")
		}
		if !strings.Contains(err.Error(), "clock is not set") {
			t.Fatalf("error does not name the clock: %v", err)
		}
	})

	t.Run("garbage envelope", func(t *testing.T) {
		if _, err := v.Verify(name, []byte("nope"), sum); err == nil {
			t.Fatal("garbage envelope accepted")
		}
	})

	t.Run("bad issuer", func(t *testing.T) {
		broken := NewVerifier([]byte("not pem"), fixedClock(goodNow))
		if _, err := broken.Verify(name, env, sum); err == nil {
			t.Fatal("verify succeeded with an unparsable issuer")
		}
	})
}

func TestEmbeddedPinsParse(t *testing.T) {
	if defaultVerifier.issuerErr != nil || defaultVerifier.issuer == nil {
		t.Fatalf("embedded firmware CA does not parse: %v", defaultVerifier.issuerErr)
	}
	if got := defaultVerifier.issuer.Subject.CommonName; got != "firmware" {
		t.Fatalf("pinned issuer CN = %q, want firmware", got)
	}
	pool := TLSConfig().RootCAs
	if pool == nil {
		t.Fatal("no root pool")
	}
}

func TestRequireHTTPS(t *testing.T) {
	if err := RequireHTTPS("http://example.com/x"); err == nil {
		t.Fatal("http accepted")
	}
	if err := RequireHTTPS("https://example.com/x"); err != nil {
		t.Fatal(err)
	}
}

func TestReadEnvelopeCap(t *testing.T) {
	if _, err := ReadEnvelope(strings.NewReader(strings.Repeat("a", MaxEnvelopeSize+1))); err == nil {
		t.Fatal("oversized envelope accepted")
	}
	if _, err := ReadEnvelope(strings.NewReader("")); err == nil {
		t.Fatal("empty envelope accepted")
	}
}
