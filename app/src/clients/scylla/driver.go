package scylla

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/rs/zerolog"
)

// @FIXME Use high performant clients for PostgreSQL, ScyllaDB, Redis, Elastic, Neo4j

// @FIXME Scylladb always use prepared statements because they are token aware
// @FIXME Aim for a single query to return the data you need, avoid multiple queries, because you might hit nodes that could be down
// 			We need to denormalize a lot to do only a single query
// @FIXME Use BATCH only for atomicity, not for performance, because it is not performant in ScyllaDB
//		   Actually DO NOT USE BATCH at all, because it is not performant in ScyllaDB
// @FIXME Use lightweight transactions (LWT) only when absolutely necessary, because they are not performant in ScyllaDB
// @FIXME connect to all nodes in the cluster, not just one, to improve performance and availability via token aware routing
// @FIXME avoid using multi-partition reads: IN queries (e.g. IN (1,2,3)), they are not token aware
//        better use multiple single partition prepared statements queries in parallel

type Client struct {
	logger zerolog.Logger
	config *gocql.ClusterConfig
	Driver *gocql.Session
}

type ClientLoggerOptions struct {
	Client zerolog.Logger
	Driver zerolog.Logger
}

type ClientOptions struct {
	Hosts             []string
	ShardAwarePort    uint16
	LocalDC           string
	Keyspace          string
	Username          string
	Password          string
	AddressTranslator gocql.AddressTranslator
	Logger            ClientLoggerOptions
}

func NewClient(options ClientOptions) *Client {
	clusterConfig := gocql.NewCluster(options.Hosts...)

	// Set up the host selection policy to be token-aware with a DC-aware fallback.
	var fallback gocql.HostSelectionPolicy
	if options.LocalDC != "" {
		fallback = gocql.DCAwareRoundRobinPolicy(options.LocalDC)
	} else {
		fallback = gocql.RoundRobinHostPolicy()
	}
	clusterConfig.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(fallback)

	// Set up shard aware port
	clusterConfig.Port = int(options.ShardAwarePort)

	// Set the default consistency level for queries.
	if options.LocalDC != "" {
		clusterConfig.Consistency = gocql.LocalQuorum
		clusterConfig.SerialConsistency = gocql.LocalSerial
	} else {
		clusterConfig.Consistency = gocql.Quorum
		clusterConfig.SerialConsistency = gocql.Serial
	}

	// Set the keyspace your application will use.
	clusterConfig.Keyspace = options.Keyspace

	// Enable compression to reduce bandwidth usage.
	clusterConfig.Compressor = &gocql.SnappyCompressor{}

	// Set the authenticator if provided.
	clusterConfig.Authenticator = gocql.PasswordAuthenticator{
		Username: options.Username,
		Password: options.Password,
	}
	clusterConfig.AddressTranslator = options.AddressTranslator

	// Resiliency
	clusterConfig.DefaultIdempotence = true
	clusterConfig.Timeout = 3 * time.Second
	clusterConfig.WriteTimeout = 3 * time.Second
	clusterConfig.ReadTimeout = 4 * time.Second
	clusterConfig.ConnectTimeout = 5 * time.Second
	clusterConfig.DisableSkipMetadata = false // Re-enable the performance optimization

	// Set up logging
	clusterConfig.Logger = &zerologAdapter{logger: options.Logger.Driver}

	return &Client{
		logger: options.Logger.Client,
		config: clusterConfig,
		Driver: nil,
	}
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return fmt.Errorf("scylla driver already started")
	}

	session, err := c.config.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create scylla session: %v", err)
	}

	c.Driver = session
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("ScyllaDB client already stopped")
		return
	}

	c.Driver.Close()
	c.Driver = nil
}

type zerologAdapter struct {
	logger zerolog.Logger
}

func (a *zerologAdapter) Print(v ...interface{}) {
	a.detectLevel(v).Msg(fmt.Sprint(v...))
}

func (a *zerologAdapter) Printf(format string, v ...interface{}) {
	a.detectLevel(v).Msgf(format, v...)
}

func (a *zerologAdapter) Println(v ...interface{}) {
	a.Print(v...)
}

func (a *zerologAdapter) detectLevel(v []interface{}) *zerolog.Event {
	if len(v) == 0 {
		return a.logger.Info()
	}

	first, ok := v[0].(string)
	if !ok {
		first = fmt.Sprint(v[0])
	}

	switch {
	case strings.HasPrefix(first, "trace"):
		return a.logger.Trace()
	case strings.HasPrefix(first, "debug"):
		return a.logger.Debug()
	case strings.HasPrefix(first, "info"):
		return a.logger.Info()
	case strings.HasPrefix(first, "warn"):
		return a.logger.Warn()
	case strings.HasPrefix(first, "error"),
		strings.HasPrefix(first, "gocql"):
		return a.logger.Error()
	case strings.HasPrefix(first, "fatal"),
		strings.HasPrefix(first, "panic"):
		return a.logger.Fatal()
	default:
		return a.logger.Info()
	}
}
