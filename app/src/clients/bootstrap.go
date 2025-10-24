package clients

import (
	"chat/src/clients/elasticsearch"
	"chat/src/clients/kafka"
	"chat/src/clients/neo4j"
	"chat/src/clients/postgresql"
	"chat/src/clients/redis"
	"chat/src/clients/scylla"
	"chat/src/platform/config"
	"chat/src/platform/logging"
	"chat/src/util"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/samber/oops"
)

const connectionTimeout = 5 * time.Second

type Closeable interface {
	Close()
}

type KafkaConsumerClients struct {
	Individual *kafka.Client
	Group      *kafka.Client
}

type KafkaClients struct {
	Admin    *kafka.Client
	Producer *kafka.Client
	Consumer KafkaConsumerClients
}

type StorageClients struct {
	Elasticsearch *elasticsearch.Client
	Neo4j         *neo4j.Client
	PostgreSQL    *postgresql.Client
	Redis         *redis.Client
	ScyllaDB      *scylla.Client
	Kafka         KafkaClients
}

var closeablesG []Closeable

func BootstrapStorageClients(config *config.Config, loggerFactory *logging.LoggerFactory) (*StorageClients, error) {
	errorb := oops.In(util.GetFunctionName())

	var err error = nil
	defer func() {
		if err != nil {
			Shutdown()
		}
	}()

	// Elasticsearch Client
	tlsConfig, err := util.CreateTLSConfigWithRootCA(config.Elasticsearch.CACertFilePath)
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create tls config for elasticsearch client")
	}

	elasticsearchClient, err := elasticsearch.NewClient(elasticsearch.ClientOptions{
		Logger: elasticsearch.ClientLoggerOptions{
			Client: loggerFactory.Child("client.elasticsearch"),
			Driver: loggerFactory.Child("client.elasticsearch.driver"),
		},
		TLSConfig:    tlsConfig,
		Username:     config.Elasticsearch.Username,
		Password:     string(config.Elasticsearch.Password),
		Addresses:    config.Elasticsearch.Addresses,
		ShouldLogReq: config.Elasticsearch.ShouldLogReq,
		ShouldLogRes: config.Elasticsearch.ShouldLogRes,
	})
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create elasticsearch client")
	}
	closeablesG = append(closeablesG, elasticsearchClient)

	// Neo4j Client
	tlsConfig, err = util.CreateTLSConfigWithRootCA(config.Neo4j.CACertFilePath)
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create tls config for neo4j client")
	}

	neo4jClient, err := neo4j.NewClient(neo4j.ClientOptions{
		Logger: neo4j.ClientLoggerOptions{
			Client:  loggerFactory.Child("client.neo4j"),
			Driver:  loggerFactory.Child("client.neo4j.driver"),
			Session: loggerFactory.Child("client.neo4j.session"),
		},
		Uri:            config.Neo4j.Uri,
		TlsConfig:      tlsConfig,
		Username:       config.Neo4j.Username,
		Password:       string(config.Neo4j.Password),
		ConnectTimeout: connectionTimeout,
		DatabaseName:   config.Neo4j.DatabaseName,
	})
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create neo4j client")
	}
	closeablesG = append(closeablesG, neo4jClient)

	// PostgreSQL Client
	postgresClient, err := postgresql.NewClient(postgresql.ClientOptions{
		URL: fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslrootcert=%s sslmode=verify-full",
			config.PostgreSQL.Username,
			string(config.PostgreSQL.Password),
			config.PostgreSQL.Host,
			config.PostgreSQL.Port,
			config.PostgreSQL.DbName,
			config.PostgreSQL.CACertFilePath,
		),
		ConnectTimeout:          connectionTimeout,
		ApplicationInstanceName: config.Application.InstanceName,
		PreparedStatements:      nil,
		Logger:                  loggerFactory.Child("client.postgresql"),
	})
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create postgresql client")
	}
	closeablesG = append(closeablesG, postgresClient)

	// Redis Client
	tlsConfig, err = util.CreateTLSConfigWithRootCA(config.Redis.CACertFilePath)
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create tls config for redis client")
	}
	cert, err := tls.LoadX509KeyPair(config.Redis.MTLSCertFilePath, config.Redis.MTLSKeyFilePath)
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to load X509 Key Pair for redis client")
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.MinVersion = tls.VersionTLS13

	redisClient, err := redis.NewClient(redis.ClientOptions{
		Addresses:  config.Redis.Addresses,
		TLSConfig:  tlsConfig,
		ClientName: config.Application.InstanceName,
		Username:   config.Redis.Username,
		Password:   string(config.Redis.Password),
		Logger:     loggerFactory.Child("client.redis"),
	})
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create redis client")
	}
	closeablesG = append(closeablesG, redisClient)

	// ScyllaDB Client
	scyllaClient, err := scylla.NewClient(scylla.ClientOptions{
		Hosts:          config.ScyllaDB.Hosts,
		ShardAwarePort: config.ScyllaDB.ShardAwarePort,
		LocalDC:        config.ScyllaDB.LocalDC,
		Keyspace:       config.ScyllaDB.Keyspace,
		Username:       config.ScyllaDB.Username,
		Password:       string(config.ScyllaDB.Password),
		Logger:         loggerFactory.Child("client.scylladb"),
	})
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to create scylladb client")
	}
	closeablesG = append(closeablesG, scyllaClient)

	// @FIXME Kafka Clients

	return &StorageClients{
		Elasticsearch: elasticsearchClient,
		Neo4j:         neo4jClient,
		PostgreSQL:    postgresClient,
		Redis:         redisClient,
		ScyllaDB:      scyllaClient,
		Kafka: KafkaClients{
			Admin:    nil, // @FIXME create kafka admin client
			Producer: nil, // @FIXME create kafka admin client
			Consumer: KafkaConsumerClients{
				Individual: nil, // @FIXME create kafka admin client
				Group:      nil, // @FIXME create kafka admin client
			},
		},
	}, nil
}

func Shutdown() {
	for i := len(closeablesG) - 1; i >= 0; i-- {
		closeablesG[i].Close()
	}
}
