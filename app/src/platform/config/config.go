package config

import (
	"chat/src/util"
)

type CredentialsConfig struct {
	Username string      `koanf:"username" validate:"required,min=4,max=64"`
	Password util.Secret `koanf:"password" validate:"required,min=4,max=64"`
}

type TlsPathsConfig struct {
	Truststore  util.Secret `koanf:"truststore"`
	Certificate util.Secret `koanf:"certificate"`
	Key         util.Secret `koanf:"key"`
}

type EtcdConfig struct {
	TlsPathsConfig `koanf:",squash"`
	Endpoints      []string `koanf:"endpoints" validate:"required,min=1,max=10,unique,dive,required,https_url"`
}

type PostgreSQLConfig struct {
	CredentialsConfig `koanf:",squash"`
	TlsPathsConfig    `koanf:",squash"`
	Host              string            `koanf:"host" validate:"required,hostname|ip"`
	Port              uint16            `koanf:"port" validate:"required,port"`
	DbName            string            `koanf:"dbname" validate:"required,min=4,max=64"`
	Options           map[string]string `koanf:"options" validate:"dive,keys,required,min=4,max=64,endkeys,required,min=1,max=64"`
}

type ScyllaDBConfig struct {
	CredentialsConfig `koanf:",squash"`
	TlsPathsConfig    `koanf:",squash"`
	Hosts             []string `koanf:"hosts" validate:"required,min=1,max=10,unique,dive,required,hostname|ip"`
	ShardAwarePort    uint16   `koanf:"shard_aware_port" validate:"required,port"`
	LocalDC           string   `koanf:"local_dc" validate:"omitempty,min=3,max=64,alphanum"`
	Keyspace          string   `koanf:"keyspace" validate:"required,min=4,max=64"`
}

type RedisConfig struct {
	CredentialsConfig `koanf:",squash"`
	TlsPathsConfig    `koanf:",squash"`
	Addresses         []string `koanf:"addresses" validate:"required,min=1,max=10,unique,dive,required,hostname_port"`
}

type ElasticsearchConfig struct {
	CredentialsConfig `koanf:",squash"`
	TlsPathsConfig    `koanf:",squash"`
	Addresses         []string `koanf:"addresses" validate:"required,min=1,max=10,unique,dive,required,http_url|https_url"`
	ShouldLogReq      bool     `koanf:"should_log_req"`
	ShouldLogRes      bool     `koanf:"should_log_res"`
}

type Neo4jConfig struct {
	CredentialsConfig `koanf:",squash"`
	TlsPathsConfig    `koanf:",squash"`
	Uri               string `koanf:"uri" validate:"required,uri,startswith=neo4j"`
	DatabaseName      string `koanf:"database_name" validate:"required,min=4,max=64,alphanum"`
}

type KafkaConfig struct {
	TlsPathsConfig `koanf:",squash"`
	SeedBrokers    []string          `koanf:"seed_brokers" validate:"required,min=1,max=10,unique,dive,required,hostname_port"`
	Users          KafkaUsers        `koanf:"users" validate:"required"`
	Topics         KafkaConfigTopics `koanf:"topics" validate:"required"`
	GroupID        string            `koanf:"group_id" validate:"required,min=4,max=64,alphanum"`
}

type KafkaUsers struct {
	Admin CredentialsConfig `koanf:"admin" validate:"required"`
	Data  CredentialsConfig `koanf:"data" validate:"required"`
}

type KafkaConfigTopics struct {
	UserInbox         string `koanf:"user_inbox" validate:"required,min=4,max=64"`
	GroupInbox        string `koanf:"group_inbox" validate:"required,min=4,max=64"`
	UserNotifications string `koanf:"user_notifications" validate:"required,min=4,max=64"`
}

type LoggingConfig struct {
	RootLevel     string            `koanf:"root_level" validate:"required,oneof=trace debug info warn error fatal panic disabled"`
	LiteralLevels map[string]string `koanf:"literal_levels" validate:"max=100,dive,keys,required,min=1,max=100,endkeys,required,oneof=trace debug info warn error fatal panic disabled"`
	RegexLevels   map[string]string `koanf:"regex_levels" validate:"max=100,dive,keys,required,min=1,max=100,endkeys,required,oneof=trace debug info warn error fatal panic disabled"`
	PrettyPrint   bool              `koanf:"pretty_print"`
}

type ApplicationConfig struct {
	TlsPathsConfig `koanf:",squash"`
	InstanceName   string
	Version        string
	Commit         string
	BuildTime      string
}

type Config struct {
	Application   ApplicationConfig   `koanf:"application" validate:"required"`
	Etcd          EtcdConfig          `koanf:"etcd" validate:"required"`
	PostgreSQL    PostgreSQLConfig    `koanf:"postgresql" validate:"required"`
	ScyllaDB      ScyllaDBConfig      `koanf:"scylladb" validate:"required"`
	Redis         RedisConfig         `koanf:"redis" validate:"required"`
	Elasticsearch ElasticsearchConfig `koanf:"elasticsearch" validate:"required"`
	Neo4j         Neo4jConfig         `koanf:"neo4j" validate:"required"`
	Kafka         KafkaConfig         `koanf:"kafka" validate:"required"`
	Logging       LoggingConfig       `koanf:"logging" validate:"required"`
}
