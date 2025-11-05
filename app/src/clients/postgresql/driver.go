package postgresql

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"github.com/twpayne/go-geos"

	pgxuuid "github.com/jackc/pgx-gofrs-uuid"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	pgxgeos "github.com/twpayne/pgx-geos"
	pgxgoogleuuid "github.com/vgarvardt/pgx-google-uuid/v5"
)

// -- @FIXME: make sure generated code by sqlc uses pgx.CollectRows https://youtu.be/sXMSWhcHCf8?si=mSZk_pq9MIG6GGR0&t=1014

type Client struct {
	logger zerolog.Logger
	config *pgxpool.Config
	Driver *pgxpool.Pool
}

type ClientOptions struct {
	URL                     string
	ApplicationInstanceName string
	PreparedStatements      *map[string]string
	TLSConfig               *tls.Config
	Logger                  zerolog.Logger
}

func NewClient(options ClientOptions) (*Client, error) {
	errorb := oops.
		In("postgresql client").
		Tags("constructor")

	config, err := pgxpool.ParseConfig(options.URL)
	if err != nil {
		return nil, errorb.Wrapf(err, "failed to parse database url")
	}

	config.MaxConns = int32(100)
	config.MinIdleConns = int32(20)
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnLifetimeJitter = 5 * time.Minute
	config.MaxConnIdleTime = 10 * time.Minute
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	config.ConnConfig.TLSConfig = options.TLSConfig
	config.ConnConfig.RuntimeParams["application_name"] = options.ApplicationInstanceName
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
		if (options.PreparedStatements != nil) && (len(*options.PreparedStatements) > 0) {
			for name, sql := range *options.PreparedStatements {
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

	return &Client{
		logger: options.Logger,
		config: config,
		Driver: nil,
	}, nil
}

func (c *Client) Start(ctx context.Context) error {
	if c.Driver != nil {
		return errors.New("postgresql client already started")
	}

	pool, err := pgxpool.NewWithConfig(ctx, c.config)
	if err != nil {
		return fmt.Errorf("failed to start postgresql client: %v", err)
	}

	c.Driver = pool
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("PostgreSQL client already stopped")
		return
	}

	c.Driver.Close()
	c.Driver = nil
}
