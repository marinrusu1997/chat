package util

import (
	error2 "chat/src/platform/error"
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/samber/oops"
)

func CreateTLSConfigWithRootCA(caCertFilePath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertFilePath)
	if err != nil {
		return nil, oops.
			In(GetFunctionName()).
			Code(error2.ENOENT).
			Wrapf(err, "failed to read CA certificate from path '%s'", caCertFilePath)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, oops.
			In(GetFunctionName()).
			Code(error2.EIO).
			Wrapf(err, "failed to appent CA certificate '%s' to cert pool", caCertFilePath)
	}

	return &tls.Config{
		RootCAs: caCertPool,
	}, nil
}
