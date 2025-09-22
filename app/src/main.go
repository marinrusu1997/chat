package main

import (
	"chat/src/postgres"
	"chat/src/postgres/gen"
	"chat/src/redis"
	"chat/src/scylla"
	"context"
	"fmt"
	"log"

	"github.com/gocql/gocql"
	redis2 "github.com/redis/go-redis/v9"
)

func main() {
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
	fmt.Printf("Scylla all good\n")

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
			log.Printf("Redis close error: %v", err)
		}
	}(redisClient)
	val, err := redisClient.Get(context.Background(), "chat:1").Result()
	if err == redis2.Nil {
		fmt.Println("Redis GET chat:1 = <nil>")
	} else if err != nil {
		log.Printf("Redis GET error: %v", err)
	} else {
		fmt.Printf("Redis GET chat:1 = %s\n", val)
	}

	// Postgres
	ctx := context.Background()
	dbURL := "postgres://chat_rw:bR4--RqiFyNQGZZiLG4e@pgpool:9999/chat_db"
	pool, err := postgres.CreatePool(ctx, dbURL, nil)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	fmt.Println("Successfully connected to PostgreSQL!")

	queries := gen.New(pool)

	createUserParams := gen.CreateUserParams{
		Name:         "John Doe",
		Email:        "john.doe@example.com",
		PasswordHash: "a_very_long_and_secure_password_hash_that_is_at_least_50_chars",
		PasswordAlgo: 1,
	}

	fmt.Printf("Deleting user '%s'...\n", createUserParams.Email)
	err = queries.DeleteUserByEmail(ctx, createUserParams.Email)
	if err != nil {
		log.Printf("Failed to delete user: %v", err)
	}

	fmt.Printf("Creating user '%s'...\n", createUserParams.Email)
	newUser, err := queries.CreateUser(ctx, createUserParams)
	if err != nil {
		log.Fatalf("Failed to create user: %v\n", err)
	}

	// 4. --- PRINTING THE RESULT ---
	// The 'newUser' variable is a type-safe struct matching your database schema.
	fmt.Println("User created successfully!!!!")
	fmt.Printf("ID:        %s\n", newUser.ID.String())
	fmt.Printf("Name:      %s\n", newUser.Name)
	fmt.Printf("Email:     %s\n", newUser.Email)
	fmt.Printf("CreatedAt: %s\n", newUser.CreatedAt.Time.String())
}
