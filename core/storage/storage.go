package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rubixchain/rubixgoplatform/types"
)

type RubixDB struct {
	pool *pgxpool.Pool
}

func GetRubixDBConnectionString(dbConfig *types.DBConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.DBName,
	)
}

// DBOpts configures the pgxpool connection pool.
type DBOpts struct {
	MaxConns                  int
	MinConns                  int
	MaxConnLifetimeInSeconds  time.Duration
	MaxConnIdleTimeInSeconds  time.Duration
	StatementTimeoutInSeconds time.Duration
}

func NewRubixDB(ctx context.Context, dbConfig *types.DBConfig, opts DBOpts) (*RubixDB, error) {
	connStr := GetRubixDBConnectionString(dbConfig)

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	// Pool tuning (adjust depending on workload)
	config.MaxConns = int32(opts.MaxConns)
	config.MinConns = int32(opts.MinConns)
	config.MaxConnLifetime = opts.MaxConnLifetimeInSeconds
	config.MaxConnIdleTime = opts.MaxConnIdleTimeInSeconds

	if opts.StatementTimeoutInSeconds > 0 {
		config.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", opts.StatementTimeoutInSeconds.Milliseconds())
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	// Verify DB connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return &RubixDB{
		pool: pool,
	}, nil
}

func (r *RubixDB) Pool() *pgxpool.Pool {
	return r.pool
}

func (r *RubixDB) Close() {
	r.pool.Close()
}

func (r *RubixDB) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *RubixDB) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}
