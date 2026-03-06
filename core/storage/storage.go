package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rubixchain/rubixgoplatform/types"
)

type RubixDB struct {
	pool *pgxpool.Pool
}

func GetRubixDBConnectionString(dbConfig *types.DBConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbConfig.DBAddress,
		dbConfig.DBPort,
		dbConfig.DBUserName,
		dbConfig.DBPassword,
		dbConfig.DBName,
	)
}

// PoolOptions configures the pgxpool connection pool.
type PoolOptions struct {
    MaxConns         int32
    MinConns         int32
    MaxConnLifetime  time.Duration
    MaxConnIdleTime  time.Duration
    StatementTimeout time.Duration
}

// DefaultPoolOptions returns production-safe defaults.
// These can be overridden via environment variables or config file.
func DefaultPoolOptions() PoolOptions {
    return PoolOptions{
        MaxConns:         50,
        MinConns:         5,
        MaxConnLifetime:  1 * time.Hour,
        MaxConnIdleTime:  10 * time.Minute,
        StatementTimeout: 5 * time.Second,
    }
}

func NewRubixDB(ctx context.Context, dbConfig *types.DBConfig, opts PoolOptions) (*RubixDB, error) {
	connStr := GetRubixDBConnectionString(dbConfig)

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	// Pool tuning (adjust depending on workload)
	config.MaxConns = opts.MaxConns
	config.MinConns = opts.MinConns
	config.MaxConnLifetime = opts.MaxConnLifetime
	config.MaxConnIdleTime = opts.MaxConnIdleTime

	if opts.StatementTimeout > 0 {
        config.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", opts.StatementTimeout.Milliseconds())
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