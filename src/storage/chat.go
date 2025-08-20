package storage

import (
	"chat/src/domain/models"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type DbChatInsert struct {
	Name         string
	Description  string
	IsGroup      bool
	IsPrivate    bool
	IsInviteOnly bool
	CreatedBy    uint64
}

func InsertChat(dbChatInsert *DbChatInsert, db *sqlx.DB) (models.ChatRow, error) {
	var chat models.ChatRow
	err := db.QueryRowx(`
		INSERT INTO chat(name, description, is_group, is_private, is_invite_only, created_by)   
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING *
	`, dbChatInsert.Name, dbChatInsert.Description, dbChatInsert.IsGroup, dbChatInsert.IsPrivate, dbChatInsert.IsInviteOnly, dbChatInsert.CreatedBy).StructScan(&chat)

	if err != nil {
		return chat, fmt.Errorf("failed to insert chat created by user with id '%d' into database: %w", dbChatInsert.CreatedBy, err)
	}
	return chat, nil
}

type DbChatUpdate struct {
	ID        uint64
	IsDeleted *bool
}
