package state

import (
	"chat/src/clients/elasticsearch"
	"chat/src/clients/email"
	"chat/src/clients/etcd"
	"chat/src/clients/kafka"
	"chat/src/clients/nats"
	"chat/src/clients/neo4j"
	"chat/src/clients/postgresql"
	"chat/src/clients/redis"
	"chat/src/clients/scylla"
	"chat/src/platform/config"
	"chat/src/platform/logging"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/emersion/go-sasl"
)

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
	Etcd          *etcd.Client
	Elasticsearch *elasticsearch.Client
	Neo4j         *neo4j.Client
	PostgreSQL    *postgresql.Client
	Redis         *redis.Client
	ScyllaDB      *scylla.Client
	Nats          *nats.Client
	Email         *email.Client
	Kafka         KafkaClients
}

func CreateClients(config *config.Config, tlsConfig map[string]*tls.Config, loggerFactory *logging.LoggerFactory) (*StorageClients, error) {
	// Elasticsearch Client
	elasticsearchClient := elasticsearch.NewClient(&elasticsearch.ClientOptions{
		Addresses:    config.Elasticsearch.Addresses,
		TLSConfig:    tlsConfig[elasticsearch.PingTargetName],
		Username:     config.Elasticsearch.Username,
		Password:     string(config.Elasticsearch.Password),
		ShouldLogReq: config.Elasticsearch.ShouldLogReq,
		ShouldLogRes: config.Elasticsearch.ShouldLogRes,
		Logger: elasticsearch.ClientLoggerOptions{
			Client: loggerFactory.Child("client.elasticsearch"),
			Driver: loggerFactory.Child("client.elasticsearch.driver"),
		},
	})

	// Neo4j Client
	neo4jClient := neo4j.NewClient(&neo4j.ClientOptions{
		URI:          config.Neo4j.URI,
		TLSConfig:    tlsConfig[neo4j.PingTargetName],
		Username:     config.Neo4j.Username,
		Password:     string(config.Neo4j.Password),
		DatabaseName: config.Neo4j.DatabaseName,
		Logger: neo4j.ClientLoggerOptions{
			Client:  loggerFactory.Child("client.neo4j"),
			Driver:  loggerFactory.Child("client.neo4j.driver"),
			Session: loggerFactory.Child("client.neo4j.session"),
		},
	})

	// PostgreSQL Client
	postgresClient, err := postgresql.NewClient(&postgresql.ClientOptions{
		URL: fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s",
			config.PostgreSQL.Username,
			string(config.PostgreSQL.Password),
			config.PostgreSQL.Host,
			config.PostgreSQL.Port,
			config.PostgreSQL.DBName,
		),
		TLSConfig:               tlsConfig[postgresql.PingTargetName],
		ApplicationInstanceName: config.Application.InstanceName,
		PreparedStatements:      nil,
		Logger:                  loggerFactory.Child("client.postgresql"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgresql client: %v", err)
	}

	// Redis Client
	redisClient := redis.NewClient(&redis.ClientOptions{
		Addresses:  config.Redis.Addresses,
		TLSConfig:  tlsConfig[redis.PingTargetName],
		Username:   config.Redis.Username,
		Password:   string(config.Redis.Password),
		ClientName: config.Application.InstanceName,
		Logger:     loggerFactory.Child("client.redis"),
	})

	// Etcd Client
	etcdClient := etcd.NewClient(&etcd.ClientOptions{
		Endpoints: config.Etcd.Endpoints,
		TLSConfig: tlsConfig[etcd.PingTargetName],
		Logger: etcd.ClientLoggerOptions{
			Client: loggerFactory.Child("client.etcd"),
			Driver: loggerFactory.Child("client.etcd.driver"),
		},
	})

	// ScyllaDB Client
	scyllaClient := scylla.NewClient(&scylla.ClientOptions{
		Hosts:          config.ScyllaDB.Hosts,
		ShardAwarePort: config.ScyllaDB.ShardAwarePort,
		TLSConfig:      tlsConfig[scylla.PingTargetName],
		LocalDC:        config.ScyllaDB.LocalDC,
		Username:       config.ScyllaDB.Username,
		Password:       string(config.ScyllaDB.Password),
		Keyspace:       config.ScyllaDB.Keyspace,
		Logger: scylla.ClientLoggerOptions{
			Client: loggerFactory.Child("client.scylla"),
			Driver: loggerFactory.Child("client.scylla.driver"),
		},
	})

	// Nats Client
	natsClient := nats.NewClient(&nats.ClientOptions{
		Servers:    config.Nats.Servers,
		TLSConfig:  tlsConfig[nats.PingTargetName],
		ClientName: config.Application.InstanceName,
		Username:   config.Nats.Username,
		Password:   string(config.Nats.Password),
		Logger:     loggerFactory.Child("client.nats"),
	})

	// Email Client
	emailClient := email.NewClient(&email.ClientOptions{
		WorkerPoolOptions: email.WorkerPoolOptions{
			SMTPClientOptions: &email.SMTPClientOptions{
				Host:              config.Email.SMTPHost,
				Port:              config.Email.SMTPPort,
				TLSConfig:         tlsConfig[email.PingTargetName],
				Auth:              sasl.NewLoginClient(config.Email.Username, string(config.Email.Password)),
				ReconnectTimeout:  5 * time.Second,
				CommandTimeout:    10 * time.Second,
				SubmissionTimeout: 15 * time.Second,
				SendTimeout:       60 * time.Second,
				Logger:            nil,
			},
			Logger:     loggerFactory.ChildPtr("client.email"),
			NumWorkers: config.Email.NumWorkers,
			QueueSize:  config.Email.QueueSize,
		}})

	// @FIXME Kafka Clients

	return &StorageClients{
		Elasticsearch: elasticsearchClient,
		Neo4j:         neo4jClient,
		Etcd:          etcdClient,
		PostgreSQL:    postgresClient,
		Redis:         redisClient,
		ScyllaDB:      scyllaClient,
		Nats:          natsClient,
		Email:         emailClient,
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
