package config

import (
	"chat/src/util"
)

type CredentialsConfig struct {
	Username string      `koanf:"username" validate:"required,min=4,max=64"`
	Password util.Secret `koanf:"password" validate:"required,min=4,max=64"`
}

type PostgreSQLConfig struct {
	CredentialsConfig `koanf:",squash"`
	Host              string            `koanf:"host" validate:"required,hostname|ip"`
	Port              uint16            `koanf:"port" validate:"required,port"`
	DbName            string            `koanf:"dbname" validate:"required,min=4,max=64"`
	CACertFilePath    string            `koanf:"ca_cert_file_path" validate:"required,filepath"`
	Options           map[string]string `koanf:"options" validate:"dive,keys,required,min=4,max=64,endkeys,required,min=1,max=64"`
}

type ScyllaDBConfig struct {
	CredentialsConfig `koanf:",squash"`
	Hosts             []string `koanf:"hosts" validate:"required,min=1,max=10,unique,dive,required,hostname|ip"`
	ShardAwarePort    uint16   `koanf:"shard_aware_port" validate:"required,port"`
	LocalDC           string   `koanf:"local_dc" validate:"omitempty,min=3,max=64,alphanum"`
	Keyspace          string   `koanf:"keyspace" validate:"required,min=4,max=64"`
}

type RedisConfig struct {
	CredentialsConfig `koanf:",squash"`
	Addresses         []string `koanf:"addresses" validate:"required,min=1,max=10,unique,dive,required,hostname_port"`
	CACertFilePath    string   `koanf:"ca_cert_file_path" validate:"required,filepath"`
	MTLSCertFilePath  string   `koanf:"mtls_cert_file_path" validate:"required,filepath"`
	MTLSKeyFilePath   string   `koanf:"mtls_key_file_path" validate:"required,filepath"`
}

type ElasticsearchConfig struct {
	CredentialsConfig `koanf:",squash"`
	Addresses         []string `koanf:"addresses" validate:"required,min=1,max=10,unique,dive,required,http_url|https_url"`
	CACertFilePath    string   `koanf:"ca_cert_file_path" validate:"required,filepath"`
	ShouldLogReq      bool     `koanf:"should_log_req"`
	ShouldLogRes      bool     `koanf:"should_log_res"`
}

type Neo4jConfig struct {
	CredentialsConfig `koanf:",squash"`
	Uri               string `koanf:"uri" validate:"required,uri,startswith=neo4j"`
	CACertFilePath    string `koanf:"ca_cert_file_path" validate:"required,filepath"`
	DatabaseName      string `koanf:"database_name" validate:"required,min=4,max=64,alphanum"`
}

type KafkaConfig struct {
	SeedBrokers    []string          `koanf:"seed_brokers" validate:"required,min=1,max=10,unique,dive,required,hostname_port"`
	CACertFilePath string            `koanf:"ca_cert_file_path" validate:"required,filepath"`
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
	InstanceName string
	Version      string
	Commit       string
	BuildTime    string
}

type Config struct {
	Application   ApplicationConfig
	PostgreSQL    PostgreSQLConfig    `koanf:"postgresql" validate:"required"`
	ScyllaDB      ScyllaDBConfig      `koanf:"scylladb" validate:"required"`
	Redis         RedisConfig         `koanf:"redis" validate:"required"`
	Elasticsearch ElasticsearchConfig `koanf:"elasticsearch" validate:"required"`
	Neo4j         Neo4jConfig         `koanf:"neo4j" validate:"required"`
	Kafka         KafkaConfig         `koanf:"kafka" validate:"required"`
	Logging       LoggingConfig       `koanf:"logging" validate:"required"`
}
