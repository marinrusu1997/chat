package util

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

func LoadTLSConfig(caCertFile string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, os.ErrInvalid
	}

	return &tls.Config{
		RootCAs: caCertPool,
	}, nil
}
