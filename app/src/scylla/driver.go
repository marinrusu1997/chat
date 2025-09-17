package scylla

import (
	"time"

	"github.com/gocql/gocql"
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

type SessionConfig struct {
	Hosts             []string
	ShardAwarePort    int
	LocalDC           string
	Keyspace          string
	Authenticator     gocql.Authenticator
	AddressTranslator gocql.AddressTranslator
}

func CreateSession(config SessionConfig) (*gocql.Session, error) {
	cluster := gocql.NewCluster(config.Hosts...)

	// Set up the host selection policy to be token-aware with a DC-aware fallback.
	var fallback gocql.HostSelectionPolicy
	if config.LocalDC != "" {
		fallback = gocql.DCAwareRoundRobinPolicy(config.LocalDC)
	} else {
		fallback = gocql.RoundRobinHostPolicy()
	}
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(fallback)

	// Set up shard aware port
	cluster.Port = config.ShardAwarePort

	// Set the default consistency level for queries.
	if config.LocalDC != "" {
		cluster.Consistency = gocql.LocalQuorum
		cluster.SerialConsistency = gocql.LocalSerial
	} else {
		cluster.Consistency = gocql.Quorum
		cluster.SerialConsistency = gocql.Serial
	}

	// Set the keyspace your application will use.
	cluster.Keyspace = config.Keyspace

	// Enable compression to reduce bandwidth usage.
	cluster.Compressor = &gocql.SnappyCompressor{}

	// Set the authenticator if provided.
	cluster.Authenticator = config.Authenticator
	cluster.AddressTranslator = config.AddressTranslator

	// Resiliency
	cluster.DefaultIdempotence = true
	cluster.Timeout = 3 * time.Second
	cluster.WriteTimeout = 3 * time.Second
	cluster.ReadTimeout = 4 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.DisableSkipMetadata = false // Re-enable the performance optimization

	// Create the session.
	return cluster.CreateSession()
}
