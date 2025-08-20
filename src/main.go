package main

import (
	"chat/src/storage"
	"fmt"

	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	db, err := storage.New("./storage/chat.db")
	if err != nil {
		panic(err)
	}

	user, err := storage.InsertUser(&storage.DbUserInsert{
		Email:        "john1@example.com",
		Name:         "John Doe",
		PasswordHash: "If your database supports RETURNING, you can use it directly with sqlx.Get or sqlx.Select",
	}, db)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", user)

	status := "suspended"
	user, err = storage.UpdateUser(&storage.DbUserUpdate{
		ID:     user.ID,
		Status: &status,
	}, db)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", user)
}
