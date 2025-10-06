package main

import (
	"chat/src/elasticsearch"
	"chat/src/neo4j"
	"chat/src/postgres"
	"chat/src/postgres/gen"
	"chat/src/redis"
	"chat/src/scylla"
	"context"
	"os"

	"github.com/gocql/gocql"
	j "github.com/neo4j/neo4j-go-driver/v6/neo4j"
	redis2 "github.com/redis/go-redis/v9"
	"go.elastic.co/ecszerolog"
)

// @FIXME: https://github.com/uber-go/guide/tree/master

func main() {
	logger := ecszerolog.New(os.Stdout)
	logger.Info().Str("version", "1.0.0").Msg("Application started")

	// Neo4j
	neo4jDriver, err := neo4j.CreateDriver(neo4j.Config{
		DbUri:      "neo4j://neo4j:7687",
		DbUser:     "neo4j",
		DbPassword: "xL2_RIfpD4q8nRoj4vsg",
	})
	defer func(neo4jDriver j.Driver, ctx context.Context) {
		err := neo4jDriver.Close(ctx)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to close Neo4j driver")
		}
	}(neo4jDriver, context.Background())
	logger.Info().Msg("Neo4j all good")

	// Elasticsearch
	_, err = elasticsearch.CreateClient(elasticsearch.Config{
		Addresses:      []string{"https://es-coordinating-1:9200"},
		Username:       "chat_app_user",
		Password:       "xG0-UU5v1dDoojVpWRXN",
		ShouldLogReq:   true,
		ShouldLogRes:   true,
		CACertFilePath: "/app/certs/es/ca/ca.crt",
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create Elasticsearch client")
	} else {
		logger.Info().Msg("Elasticsearch all good")
	}

	// Scylla
	session, err := scylla.CreateSession(scylla.SessionConfig{
		// Hosts:          []string{"127.0.0.1"},
		Hosts:          []string{"scylla-node1"},
		ShardAwarePort: 19042,
		LocalDC:        "DC1",
		Keyspace:       "chat_db",
		Authenticator: gocql.PasswordAuthenticator{
			Username: "chat_rw",
			Password: "yT3-4d5dQiD6S-yHfThN",
		},
		/*AddressTranslator: scylla.NewStaticAddressTranslator(map[string]string{
			"172.18.0.7:19042":  "127.0.0.1:19041",
			"172.18.0.10:19042": "127.0.0.1:19042",
			"172.18.0.9:19042":  "127.0.0.1:19043",
		}),*/
	})
	if err != nil {
		panic(err)
	}
	defer session.Close()
	logger.Info().Msg("Scylla all good")

	// Redis
	redisClient := redis.CreateClusterClient(redis.Config{
		Addresses: []string{
			"redis-node-1:6379",
			"redis-node-2:6379",
			"redis-node-3:6379",
			"redis-node-4:6379",
			"redis-node-5:6379",
			"redis-node-6:6379",
		},
		ClientName: "chat-app",
		Username:   "chat_rw",
		Password:   "Ch@t-@pp-Us€r-P@ssw0rd-!n-Pr0d",
	})
	defer func(redisClient *redis2.ClusterClient) {
		err := redisClient.Close()
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to close redis")
		}
	}(redisClient)
	val, err := redisClient.Get(context.Background(), "chat:1").Result()
	if err == redis2.Nil {
		logger.Info().Msg("Redis GET chat:1 = <nil>")
	} else if err != nil {
		logger.Warn().Err(err).Msg("Redis GET error")
	} else {
		logger.Warn().Str("chat:1", val).Msg("Redis GET:")
	}

	// Postgres
	ctx := context.Background()
	dbURL := "postgres://chat_rw:bR4--RqiFyNQGZZiLG4e@pgpool:9999/chat_db"
	pool, err := postgres.CreatePool(ctx, dbURL, nil)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	logger.Info().Msg("Postgres all good")

	queries := gen.New(pool)

	createUserParams := gen.CreateUserParams{
		Name:         "John Doe",
		Email:        "john.doe@example.com",
		PasswordHash: "a_very_long_and_secure_password_hash_that_is_at_least_50_chars",
		PasswordAlgo: 1,
	}

	logger.Info().Str("email", createUserParams.Email).Msg("Deleting user")
	err = queries.DeleteUserByEmail(ctx, createUserParams.Email)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete user")
	}

	logger.Info().Str("email", createUserParams.Email).Msg("Creating user")
	newUser, err := queries.CreateUser(ctx, createUserParams)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create user")
	}

	// 4. --- PRINTING THE RESULT ---
	// The 'newUser' variable is a type-safe struct matching your database schema.
	logger.Info().
		Str("ID", newUser.ID.String()).
		Str("Name", newUser.Name).
		Str("Email", newUser.Email).
		Str("CreatedAt", newUser.CreatedAt.Time.String()).
		Msg("Successfully created user")
}
