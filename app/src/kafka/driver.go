package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

func loadTLSConfig(caCertFile string) (*tls.Config, error) {
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

type ClientConfig struct {
	CaCertFile string
	Brokers    []string
	Username   string
	Password   string
}

func NewAdminClient(config ClientConfig) (*kadm.Client, error) {
	// 1. Load TLS configuration
	tlsCfg, err := loadTLSConfig(config.CaCertFile)
	if err != nil {
		return nil, err
	}

	// 2. Configure kgo.Client options
	opts := []kgo.Opt{
		kgo.SeedBrokers(config.Brokers...),

		kgo.SASL(plain.Auth{
			User: config.Username,
			Pass: config.Password,
		}.AsMechanism()),

		kgo.DialTLSConfig(tlsCfg),
	}

	// 3. Create the Kgo client
	kclient, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	// 4. Create the Admin client
	adminClient := kadm.NewClient(kclient)
	return adminClient, nil
}
