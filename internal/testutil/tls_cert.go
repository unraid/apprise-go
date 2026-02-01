package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var (
	testTLSOnce     sync.Once
	testTLSCert     tls.Certificate
	testTLSCertPEM  []byte
	testTLSInitErr  error
)

func TestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	cert, err := testTLSCertificate()
	if err != nil {
		t.Fatalf("generate test tls cert: %v", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func TestTLSCACertPath(t *testing.T) string {
	t.Helper()

	_, certPEM, err := testTLSCertificatePEM()
	if err != nil {
		t.Fatalf("generate test tls cert: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test-ca.pem")
	if err := os.WriteFile(path, certPEM, 0o600); err != nil {
		t.Fatalf("write test ca cert: %v", err)
	}
	return path
}

func testTLSCertificate() (tls.Certificate, error) {
	testTLSOnce.Do(func() {
		cert, pemBytes, err := generateSelfSignedCert()
		if err != nil {
			testTLSInitErr = err
			return
		}
		testTLSCert = cert
		testTLSCertPEM = pemBytes
	})
	return testTLSCert, testTLSInitErr
}

func testTLSCertificatePEM() (tls.Certificate, []byte, error) {
	cert, err := testTLSCertificate()
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return cert, testTLSCertPEM, nil
}

func generateSelfSignedCert() (tls.Certificate, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "apprise-go-test",
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return cert, certPEM, nil
}
