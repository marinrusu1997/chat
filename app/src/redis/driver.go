package redis

import (
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addresses  []string
	ClientName string
	Username   string
	Password   string
}

func CreateClusterClient(config Config) *redis.ClusterClient {
	return redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:      config.Addresses,
		ClientName: config.ClientName,
		Username:   config.Username,
		Password:   config.Password,
		NewClient: func(opt *redis.Options) *redis.Client {
			opt.DB = 0
			opt.MaxRetries = 5
			opt.ReadTimeout = 2 * time.Second
			opt.WriteTimeout = 2 * time.Second
			opt.ContextTimeoutEnabled = true
			opt.PoolFIFO = true
			opt.MinIdleConns = 10
			opt.MaxIdleConns = 50
			opt.ConnMaxLifetime = 1 * time.Hour

			return redis.NewClient(opt)
		},
		ReadOnly:       true,
		RouteByLatency: true,
	})
}
