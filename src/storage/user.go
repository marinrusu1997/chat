package storage

import (
	"chat/src/domain/models"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type DbUserInsert struct {
	Email        string
	PasswordHash string
	Name         string
}

func InsertUser(dbUserInsert *DbUserInsert, db *sqlx.DB) (models.UserRow, error) {
	var user models.UserRow
	err := db.QueryRowx(`
		INSERT INTO user(email, password_hash, name)   
		VALUES ($1, $2, $3)
		RETURNING *
	`, dbUserInsert.Email, dbUserInsert.PasswordHash, dbUserInsert.Name).StructScan(&user)

	if err != nil {
		return user, fmt.Errorf("failed to insert user with email '%s' into database: %w", dbUserInsert.Email, err)
	}
	return user, nil
}

type DbUserUpdate struct {
	ID           uint64
	PasswordHash *string
	LastLoginAt  *time.Time
	LastActiveAt *time.Time
}

func UpdateUser(dbUserUpdate *DbUserUpdate, db *sqlx.DB) (models.UserRow, error) {
	var user models.UserRow
	var queryParts []string
	var args []interface{}
	i := 1

	if dbUserUpdate.PasswordHash != nil {
		queryParts = append(queryParts, fmt.Sprintf("password_hash = $%d", i))
		args = append(args, *dbUserUpdate.PasswordHash)
		i++
	}
	if dbUserUpdate.LastLoginAt != nil {
		queryParts = append(queryParts, fmt.Sprintf("last_login_at = $%d", i))
		args = append(args, *dbUserUpdate.LastLoginAt)
		i++
	}
	if dbUserUpdate.LastActiveAt != nil {
		queryParts = append(queryParts, fmt.Sprintf("last_active_at = $%d", i))
		args = append(args, *dbUserUpdate.LastActiveAt)
		i++
	}
	if len(queryParts) == 0 {
		return user, fmt.Errorf("no fields to update for user with id '%d'", dbUserUpdate.ID)
	}
	args = append(args, dbUserUpdate.ID)

	query := fmt.Sprintf(`
        UPDATE user
        SET %s
        WHERE id = $%d
        RETURNING *
    `, strings.Join(queryParts, ", "), i)

	err := db.Get(&user, query, args...)
	return user, err
}

func DeleteUser(id uint64, db *sqlx.DB) error {
	res, err := db.Exec(`DELETE FROM user WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user with id '%d' from database: %w", id, err)
	}

	deletedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get number of deleted rows while deleting user with id '%d': %w", id, err)
	}

	if deletedRows != 1 {
		return fmt.Errorf("expected to delete 1 user with id '%d', but deleted %d", id, deletedRows)
	}

	return nil
}
