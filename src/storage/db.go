package storage

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // SQLite driver
)

func New(storagePath string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", storagePath, err)
	}
	return db, nil
}
