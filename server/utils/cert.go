package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	DefaultCertFile = "/etc/kvm/server.crt"
	DefaultKeyFile  = "/etc/kvm/server.key"

	certValidFor = time.Hour * 24 * 365 * 10
)

// GenerateCert replaces the self-signed certificate at the default paths.
func GenerateCert() error {
	return generateCert(DefaultCertFile, DefaultKeyFile)
}

// EnsureCert generates a self-signed certificate when the certificate or its
// key is missing. An existing pair is never touched.
func EnsureCert(certFile, keyFile string) error {
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)
	if certErr == nil && keyErr == nil {
		return nil
	}
	if (certErr != nil && !errors.Is(certErr, os.ErrNotExist)) ||
		(keyErr != nil && !errors.Is(keyErr, os.ErrNotExist)) {
		return errors.Join(certErr, keyErr)
	}

	log.Infof("certificate %s or key %s is missing, generating a self-signed pair", certFile, keyFile)
	return generateCert(certFile, keyFile)
}

// certNames lists the names and addresses the device answers to: its hostname,
// nanokvm.local and every interface address.
func certNames() ([]string, []net.IP) {
	dnsNames := []string{"localhost", "nanokvm.local"}
	if hostname, err := os.Hostname(); err == nil && hostname != "" && hostname != "localhost" {
		dnsNames = append(dnsNames, hostname)
	}

	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Warnf("failed to list interface addresses for the certificate: %v", err)
		return dnsNames, ips
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
			continue
		}
		ips = append(ips, ipNet.IP)
	}

	return dnsNames, ips
}

func generateCert(certFile, keyFile string) error {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Errorf("failed to generate ECDSA private key: %v", err)
		return err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Errorf("failed to generate serial number: %v", err)
		return err
	}

	dnsNames, ipAddresses := certNames()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: dnsNames[len(dnsNames)-1],
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(certValidFor),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		log.Errorf("failed to create certificate: %v", err)
		return err
	}

	privateBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		log.Errorf("failed to marshal private key: %v", err)
		return err
	}

	// the key goes first: a certificate without its key is never left behind
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateBytes})
	if err = writeCertFile(keyFile, keyPEM, 0o600); err != nil {
		log.Errorf("failed to write %s: %v", keyFile, err)
		return err
	}
	log.Debugf("%s generated", keyFile)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err = writeCertFile(certFile, certPEM, 0o644); err != nil {
		log.Errorf("failed to write %s: %v", certFile, err)
		return err
	}
	log.Debugf("%s generated", certFile)

	return nil
}

// writeCertFile creates the file with mode from the start and fixes the mode of
// one that already exists.
func writeCertFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
