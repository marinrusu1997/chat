package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geos"

	pgxuuid "github.com/jackc/pgx-gofrs-uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	pgxgeos "github.com/twpayne/pgx-geos"
	pgxgoogleuuid "github.com/vgarvardt/pgx-google-uuid/v5"
)

func CreatePool(sessionCtx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database url: %v", err)
	}

	config.MaxConns = int32(100)
	config.MinIdleConns = int32(20)
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnLifetimeJitter = 5 * time.Minute
	config.MaxConnIdleTime = 10 * time.Minute
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	config.ConnConfig.RuntimeParams["application_name"] = "chat-app"
	config.AfterConnect = func(connectionCtx context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		pgxgoogleuuid.Register(conn.TypeMap())
		pgxdecimal.Register(conn.TypeMap())
		err := pgxgeos.Register(connectionCtx, conn, geos.NewContext())
		if err != nil {
			return fmt.Errorf("failed to register 'pgxgeos' on pgx connection 'postgres://%s@%s:%d/%s': %v",
				conn.Config().User, conn.Config().Host, conn.Config().Port, conn.Config().Database, err,
			)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(sessionCtx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %v", err)
	}

	return pool, nil
}
