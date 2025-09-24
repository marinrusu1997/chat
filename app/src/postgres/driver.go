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

// -- @FIXME: make sure generated code by sqlc uses pgx.CollectRows https://youtu.be/sXMSWhcHCf8?si=mSZk_pq9MIG6GGR0&t=1014

func CreatePool(sessionCtx context.Context, databaseURL string, preparedStatements *map[string]string) (*pgxpool.Pool, error) {
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
	config.ConnConfig.RuntimeParams["timezone"] = "UTC"
	config.ConnConfig.RuntimeParams["datestyle"] = "ISO"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "5s"
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "2s"
	config.AfterConnect = func(connectionCtx context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		pgxgoogleuuid.Register(conn.TypeMap())
		pgxdecimal.Register(conn.TypeMap())
		err := pgxgeos.Register(connectionCtx, conn, geos.NewContext())
		if err != nil {
			return fmt.Errorf("failed to register 'pgxgeos' on pgx connection 'postgres://%s@%s:%d/%s' with id '%d': %v",
				conn.Config().User, conn.Config().Host, conn.Config().Port, conn.Config().Database, conn.PgConn().PID(), err,
			)
		}
		if (preparedStatements != nil) && (len(*preparedStatements) > 0) {
			for name, sql := range *preparedStatements {
				_, err := conn.Prepare(connectionCtx, name, sql)
				if err != nil {
					return fmt.Errorf("failed to prepare statement '%s' on pgx connection 'postgres://%s@%s:%d/%s' with id '%d': %v",
						name, conn.Config().User, conn.Config().Host, conn.Config().Port, conn.Config().Database, conn.PgConn().PID(), err,
					)
				}
			}
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(sessionCtx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %v", err)
	}

	return pool, nil
}
