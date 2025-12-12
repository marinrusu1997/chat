package config

import (
	"chat/src/platform/validation"
	"os"
	"strings"

	"github.com/creasty/defaults"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/samber/oops"
)

type LoadConfigOptions struct {
	YamlFilePaths []string
	EnvVarPrefix  string
}

var koanfG = koanf.NewWithConf(koanf.Conf{
	Delim:       ".",
	StrictMerge: true,
})

func Load(options LoadConfigOptions) (*Config, error) {
	errorBuilder := oops.
		In("config").
		Tags("loader")

	var cfg Config

	// 1. Set defaults
	if err := defaults.Set(&cfg); err != nil {
		return nil, errorBuilder.Wrapf(err, "failed to set config defaults")
	}

	// 2. Load config
	for _, path := range options.YamlFilePaths {
		if err := koanfG.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, errorBuilder.Wrapf(err, "failed to load config file %s", path)
		}
	}

	err := koanfG.Load(env.Provider(".", env.Opt{
		Prefix: options.EnvVarPrefix,
		TransformFunc: func(k, v string) (string, any) {
			k = strings.TrimPrefix(k, options.EnvVarPrefix)
			k = strings.NewReplacer("__", "_", "_", ".").Replace(k)
			k = strings.ToLower(k)
			return k, v
		},
	}), nil)
	if err != nil {
		return nil, errorBuilder.Wrapf(err, "failed to load environment variables")
	}

	if err := koanfG.Unmarshal("", &cfg); err != nil {
		return nil, errorBuilder.Wrapf(err, "failed to unmarshal config")
	}

	// 3. Validate config
	if err := validation.Instance.Struct(&cfg); err != nil {
		return nil, errorBuilder.Wrapf(err, "failed to validate config")
	}

	// 4. Add dynamic config
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errorBuilder.Wrapf(err, "failed to get hostname")
	}
	cfg.Application.Name = "chat-app"
	cfg.Application.InstanceName = hostname
	cfg.Application.Version = getEnv("BUILD_VERSION", "unknown")
	cfg.Application.Commit = getEnv("BUILD_COMMIT", "unknown")
	cfg.Application.BuildTime = getEnv("BUILD_TIME", "unknown")

	return &cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
