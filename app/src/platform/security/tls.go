package security

import (
	"chat/src/platform/perr"
	"chat/src/platform/validation"
	"chat/src/util"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/samber/oops"
)

type TLSMaterialPaths struct {
	Truststore  string `validate:"omitempty,nefield,max=255,filepath,endswith=.crt"`
	Certificate string `validate:"omitempty,nefield,required_with=Key,max=255,filepath,endswith=.crt"`
	Key         string `validate:"omitempty,nefield,required_with=Certificate,max=255,filepath,endswith=.key"`
}

type TLSPolicy struct {
	RequireMutualTLS bool
}

type TLSServiceOptions struct {
	Paths  TLSMaterialPaths
	Policy TLSPolicy
}

type TLSConfigSources struct {
	Global   TLSMaterialPaths             `validate:"required"`
	Services map[string]TLSServiceOptions `validate:"required,min=1,max=50,dive,keys,min=1,max=50,alphanum,lowercase"`
}

type TLSConfigs struct {
	Global   *tls.Config
	Services map[string]*tls.Config
}

func LoadTLSConfigs(sources *TLSConfigSources) (TLSConfigs, error) {
	if err := sources.setup(); err != nil {
		return TLSConfigs{}, fmt.Errorf("can't load tls configs, because sources setup failed: %w", err)
	}

	globalConfig, err := buildTLSConfig(&sources.Global)
	if err != nil {
		return TLSConfigs{}, fmt.Errorf("can't build global tls config: %w", err)
	}

	serviceConfigs := make(map[string]*tls.Config, len(sources.Services))
	for svcName, svcOptions := range sources.Services {
		svcConfig, err := buildTLSConfig(&svcOptions.Paths)
		if err != nil {
			return TLSConfigs{}, fmt.Errorf("can't build tls config for service '%s': %w", svcName, err)
		}
		serviceConfigs[svcName] = svcConfig
	}

	return TLSConfigs{
		Global:   globalConfig,
		Services: serviceConfigs,
	}, nil
}

func buildTLSConfig(paths *TLSMaterialPaths) (*tls.Config, error) {
	// 1. Load trusted CA bundle
	caBytes, err := os.ReadFile(paths.Truststore)
	if err != nil {
		return nil, oops.
			In(util.GetFunctionName()).
			Code(perr.ENOENT).
			Wrapf(err, "failed to read truststore from path '%s'", paths.Truststore)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, oops.
			In(util.GetFunctionName()).
			Code(perr.ENOENT).
			Wrapf(err, "failed to appent truststore from path '%s' to cert pool", paths.Truststore)
	}

	// 2. Load client certificate and key
	var certificates []tls.Certificate
	if paths.Certificate != "" && paths.Key != "" {
		cert, err := tls.LoadX509KeyPair(paths.Certificate, paths.Key)
		if err != nil {
			return nil, oops.
				In(util.GetFunctionName()).
				Code(perr.ENOENT).
				Wrapf(err, "failed to load certificate from path '%s' and key from path '%s': %v", paths.Certificate, paths.Key, err)
		}
		certificates = append(certificates, cert)
	}

	// 3. Create TLS config
	return &tls.Config{
		RootCAs:       caPool,
		Certificates:  certificates,
		MinVersion:    tls.VersionTLS13,
		Renegotiation: tls.RenegotiateNever,
	}, nil
}

func (c *TLSConfigSources) setup() error {
	errorb := oops.
		In(util.GetFunctionName()).
		Code(perr.ECONFIG)

	if err := validation.Instance.Struct(c); err != nil {
		return errorb.Wrapf(err, "failed to validate")
	}

	if c.Global.Truststore == "" || c.Global.Certificate == "" || c.Global.Key == "" {
		return errorb.New("global TLS material paths must be all specified") //nolint:wrapcheck // we are already wrapping it
	}

	for svcName, svcConfig := range c.Services {
		if svcConfig.Policy.RequireMutualTLS {
			if svcConfig.Paths.Truststore != "" && svcConfig.Paths.Certificate != "" && svcConfig.Paths.Key != "" {
				continue
			}
			if svcConfig.Paths.Truststore == "" && svcConfig.Paths.Certificate == "" && svcConfig.Paths.Key == "" {
				svcConfig.Paths = c.Global
				c.Services[svcName] = svcConfig
				continue
			}
			return errorb.Errorf("service '%s' requires eithr all or none of TLS material paths to be specified for mTLS", svcName)
		}

		if svcConfig.Paths.Certificate == "" && svcConfig.Paths.Key == "" {
			if svcConfig.Paths.Truststore == "" {
				svcConfig.Paths.Truststore = c.Global.Truststore
				c.Services[svcName] = svcConfig
			}
			continue
		}
		return errorb.Errorf("service '%s' requires either no TLS material paths or only truststore when mTLS is disabled: given %+v", svcName, svcConfig)
	}

	return nil
}
