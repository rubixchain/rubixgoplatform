package storage

import (
	"context"
)

func (r *RubixDB) InitSchema(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS transactions (
            id          TEXT PRIMARY KEY,
            info        JSON NOT NULL,
            signature   JSONB NOT NULL,
            created_at  TIMESTAMPTZ DEFAULT NOW(),
            updated_at  TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS tokenchain (
            token_id       TEXT   NOT NULL,
            transaction_id TEXT   NOT NULL,
            role           TEXT   NOT NULL,
            type           TEXT   NOT NULL,
            position       BIGINT NOT NULL,
            created_at     TIMESTAMPTZ DEFAULT NOW(),
            updated_at     TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (token_id, position)
        );

        CREATE TABLE IF NOT EXISTS did_algo (
            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            name TEXT NOT NULL,
            is_active BOOLEAN DEFAULT TRUE
        );

        CREATE TABLE IF NOT EXISTS dids (
            did         TEXT PRIMARY KEY,
            peer_did    TEXT,
            local       BOOLEAN DEFAULT TRUE,
            algo_id     SMALLINT,
            CONSTRAINT algo_id_fk FOREIGN KEY (algo_id) REFERENCES did_algo(id)
        );

        CREATE TABLE IF NOT EXISTS token_status (
            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            name TEXT NOT NULL,
            is_active BOOLEAN DEFAULT TRUE
        );

        CREATE TABLE IF NOT EXISTS token_type (
            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            name TEXT NOT NULL,
            is_active BOOLEAN DEFAULT TRUE
        );

        CREATE TABLE IF NOT EXISTS tokens (
            token_id         TEXT PRIMARY KEY,
            parent_token_id  TEXT REFERENCES tokens(token_id) ON DELETE SET NULL,
            token_value      NUMERIC NOT NULL,
            token_status     SMALLINT NOT NULL,
            did              TEXT NOT NULL,
            data             TEXT NOT NULL,
            memo             TEXT,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            token_type       SMALLINT NOT NULL,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),
            CONSTRAINT did_fk FOREIGN KEY (did) REFERENCES dids(did),
            CONSTRAINT transaction_id_fk FOREIGN KEY (transaction_id) REFERENCES transactions(id),
            CONSTRAINT token_status_fk FOREIGN KEY (token_status) REFERENCES token_status(id),
            CONSTRAINT token_type_fk FOREIGN KEY (token_type) REFERENCES token_type(id)
        );

        CREATE TABLE IF NOT EXISTS token_provider_map (
            token          TEXT PRIMARY KEY,
            did            TEXT,
            func_id        INTEGER,
            role           INTEGER,
            transaction_id TEXT,
            sender         TEXT,
            receiver       TEXT,
            token_value    NUMERIC,
            created_at     TIMESTAMPTZ DEFAULT NOW(),
            updated_at     TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS unpledge_sequence_info (
            tx_id          TEXT PRIMARY KEY,
            pledge_tokens  TEXT,
            epoch          INTEGER,
            quorum_did     TEXT,
            created_at     TIMESTAMPTZ DEFAULT NOW(),
            updated_at     TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS fts (
            id          TEXT PRIMARY KEY,
            ft_name     TEXT,
            ft_count    INTEGER,
            creator_did TEXT,
            created_at  TIMESTAMPTZ DEFAULT NOW(),
            updated_at  TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS token_recovery (
            transaction_id TEXT PRIMARY KEY,
            recovered_at   TIMESTAMPTZ,
            recovered_by   TEXT,
            token_count    INTEGER,
            token_ids      TEXT,
            recovery_type  TEXT,
            recovery_notes TEXT,
            created_at     TIMESTAMPTZ DEFAULT NOW(),
            updated_at     TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS local_test_token_info (
            attribute TEXT PRIMARY KEY,
            value     INTEGER,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS call_back_urls (
            smart_contract_hash TEXT PRIMARY KEY,
            callback_url        TEXT,
            created_at          TIMESTAMPTZ DEFAULT NOW(),
            updated_at          TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS token_state_hashes (
            did              TEXT,
            token_state_hash TEXT PRIMARY KEY,
            pledged_token    TEXT,
            transaction_id   TEXT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS quorum_manager (
            address TEXT PRIMARY KEY,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

		CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			transaction_id TEXT NOT NULL,
			status SMALLINT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
		);
    `)
	return err
}
