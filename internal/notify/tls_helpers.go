package notify

import (
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

func loadCertPoolFromFile(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("failed to load ca certs")
	}
	return pool, nil
}

func loadCertPoolFromEnv() (*x509.CertPool, bool, error) {
	path := strings.TrimSpace(os.Getenv("SSL_CERT_FILE"))
	if path == "" {
		return nil, false, nil
	}
	pool, err := loadCertPoolFromFile(path)
	if err != nil {
		return nil, true, err
	}
	return pool, true, nil
}
