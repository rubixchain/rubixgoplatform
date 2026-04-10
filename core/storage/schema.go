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

	        CREATE TABLE IF NOT EXISTS did_algo (
	            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	            name TEXT NOT NULL UNIQUE,
            is_active BOOLEAN DEFAULT TRUE
        );

	        CREATE TABLE IF NOT EXISTS dids (
	            did         TEXT PRIMARY KEY,
	            peer_id    TEXT,
	            local       BOOLEAN DEFAULT TRUE,
	            algo_id     SMALLINT,
            CONSTRAINT algo_id_fk 
            FOREIGN KEY (algo_id) 
	            REFERENCES did_algo(id)
	        );

	        CREATE TABLE IF NOT EXISTS transaction_units (
	            transaction_id  TEXT NOT NULL,
	            did             TEXT NOT NULL,
	            execution_role  TEXT NOT NULL CHECK (execution_role IN ('initiator', 'quorum', 'receiver')),
	            status          TEXT NOT NULL CHECK (status = 'committed'),
	            created_at      TIMESTAMPTZ DEFAULT NOW(),
	            updated_at      TIMESTAMPTZ DEFAULT NOW(),
	            PRIMARY KEY (transaction_id, did),
	            CONSTRAINT fk_tu_transaction
	                FOREIGN KEY (transaction_id)
	                REFERENCES transactions(id)
	                DEFERRABLE INITIALLY DEFERRED,
	            CONSTRAINT fk_tu_did
	                FOREIGN KEY (did)
	                REFERENCES dids(did)
	                DEFERRABLE INITIALLY DEFERRED
	        );

	        CREATE INDEX IF NOT EXISTS idx_transaction_units_did ON transaction_units(did);

        CREATE TABLE IF NOT EXISTS token_role (
            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            is_active BOOLEAN DEFAULT TRUE
        );

        CREATE TABLE IF NOT EXISTS token_type (
            id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            is_active BOOLEAN DEFAULT TRUE
        );

        CREATE TABLE IF NOT EXISTS tokens (
            token_id         TEXT PRIMARY KEY,
            parent_token_id  TEXT REFERENCES tokens(token_id) ON DELETE SET NULL,
            token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
            token_status     SMALLINT NOT NULL DEFAULT 99,
            did              TEXT NOT NULL,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            token_type       SMALLINT NOT NULL,
            latest_position BIGINT NOT NULL DEFAULT 0,
            latest_role SMALLINT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),
            CONSTRAINT tokens_did_fk 
            FOREIGN KEY (did) 
            REFERENCES dids(did),
            CONSTRAINT transaction_id_fk 
            FOREIGN KEY (transaction_id) 
            REFERENCES transactions(id) 
            DEFERRABLE INITIALLY DEFERRED,
            CONSTRAINT token_type_fk 
            FOREIGN KEY (token_type) 
            REFERENCES token_type(id)
        );
        CREATE INDEX IF NOT EXISTS idx_tokens_did_status ON tokens(did, token_status);


        -- moving tokenchain below so that all referenced tables are defined before it. tokenchain has FKs to transactions, token_role, and tokens.


        CREATE TABLE IF NOT EXISTS tokenchain (
	            id             INT        GENERATED ALWAYS AS IDENTITY,
	            token_id       TEXT       NOT NULL,
	            transaction_id TEXT       NOT NULL,
	            previous_transaction_id TEXT,
	            role           SMALLINT   NOT NULL,
	            position       BIGINT     NOT NULL,
	            created_at     TIMESTAMPTZ DEFAULT NOW(),
	            updated_at     TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (token_id, position),
            UNIQUE (id),
            UNIQUE(token_id, transaction_id), -- a token can only appear once in the tokenchain for a given transaction
            -- No constraint prevents orphaned tokenchain entries pointing to non-existent transactions
	            CONSTRAINT fk_tc_tx 
	                FOREIGN KEY (transaction_id) 
	                REFERENCES transactions(id) 
	                DEFERRABLE INITIALLY DEFERRED,
	            CONSTRAINT fk_tc_prev_tx
	                FOREIGN KEY (previous_transaction_id)
	                REFERENCES transactions(id)
	                DEFERRABLE INITIALLY DEFERRED,
	            -- role is SMALLINT but has no FK to token_role(id). Invalid role IDs can be inserted silently.
	            CONSTRAINT fk_tc_role 
	                FOREIGN KEY (role) 
	                REFERENCES token_role(id),
            -- A tokenchain entry can reference a token_id that doesn't exist in tokens
            CONSTRAINT fk_tc_token 
                FOREIGN KEY (token_id) 
                REFERENCES tokens(token_id) 
                DEFERRABLE INITIALLY DEFERRED,
            CONSTRAINT chk_prev_tx_rules
                CHECK (
                (position = 0 AND previous_transaction_id IS NULL)
                OR
                (position > 0 AND previous_transaction_id IS NOT NULL) 
                      )     
            );


	        CREATE TABLE IF NOT EXISTS tokenchain_index (
            token_id   TEXT PRIMARY KEY,
            index      INTEGER[] NOT NULL,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS unpledge_sequence_info (
            tx_id          TEXT PRIMARY KEY,
            pledge_tokens  TEXT[],
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
            did TEXT PRIMARY KEY,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );

		CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			transaction_id TEXT,
			status SMALLINT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
		);

        CREATE TABLE IF NOT EXISTS token_denom (
            id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            did TEXT NOT NULL,
            denom NUMERIC(4, 3) NOT NULL,
            count BIGINT NOT NULL,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW(),

            CONSTRAINT unique_did_denom UNIQUE (did, denom)
        );

        CREATE TABLE IF NOT EXISTS fullnode_transactions (
            id          TEXT PRIMARY KEY,
            info        JSON NOT NULL,
            signature   JSONB NOT NULL,
            created_at  TIMESTAMPTZ DEFAULT NOW(),
            updated_at  TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS fullnode_rbt (
            token_id         TEXT PRIMARY KEY,
            parent_token_id  TEXT,
            token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
            token_status     SMALLINT NOT NULL DEFAULT 99,
            did              TEXT NOT NULL,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            latest_position  BIGINT NOT NULL DEFAULT 0,
            latest_role      SMALLINT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),
            CONSTRAINT fullnode_rbt_transaction_id_fk 
            FOREIGN KEY (transaction_id) 
            REFERENCES fullnode_transactions(id)
            DEFERRABLE INITIALLY DEFERRED
        );

        CREATE TABLE IF NOT EXISTS fullnode_ft (
            token_id         TEXT PRIMARY KEY,
            token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
            token_status     SMALLINT NOT NULL DEFAULT 99,
            did              TEXT NOT NULL,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            latest_position  BIGINT NOT NULL DEFAULT 0,
            latest_role      SMALLINT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),
            CONSTRAINT fullnode_ft_transaction_id_fk 
            FOREIGN KEY (transaction_id) 
            REFERENCES fullnode_transactions(id)
            DEFERRABLE INITIALLY DEFERRED
        );

        CREATE TABLE IF NOT EXISTS fullnode_nft (
            token_id         TEXT PRIMARY KEY,
            token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
            token_status     SMALLINT NOT NULL DEFAULT 99,
            did              TEXT NOT NULL,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            latest_position  BIGINT NOT NULL DEFAULT 0,
            latest_role      SMALLINT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),
            CONSTRAINT fullnode_nft_transaction_id_fk   
            FOREIGN KEY (transaction_id) 
            REFERENCES fullnode_transactions(id)
            DEFERRABLE INITIALLY DEFERRED
        );

        CREATE TABLE IF NOT EXISTS fullnode_smart_contract (
            token_id         TEXT PRIMARY KEY,
            token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
            token_status     SMALLINT NOT NULL DEFAULT 99,
            transaction_id   TEXT NOT NULL,
            token_state_hash TEXT NOT NULL,
            latest_position  BIGINT NOT NULL DEFAULT 0,
            latest_role      SMALLINT,
            created_at       TIMESTAMPTZ DEFAULT NOW(),
            updated_at       TIMESTAMPTZ DEFAULT NOW(),     
            CONSTRAINT fullnode_smart_contract_transaction_id_fk 
            FOREIGN KEY (transaction_id) 
            REFERENCES fullnode_transactions(id)
            DEFERRABLE INITIALLY DEFERRED
        );

        CREATE TABLE IF NOT EXISTS fullnode_tokenchain (
            id                         INT        GENERATED ALWAYS AS IDENTITY,
            token_id                   TEXT       NOT NULL,
            transaction_id             TEXT       NOT NULL,
            previous_transaction_id    TEXT,
            role                       SMALLINT   NOT NULL,
            position                   BIGINT     NOT NULL,
            created_at                 TIMESTAMPTZ DEFAULT NOW(),
            updated_at                 TIMESTAMPTZ DEFAULT NOW(),
            PRIMARY KEY (token_id, position),
            UNIQUE (id),
            -- No constraint prevents orphaned tokenchain entries pointing to non-existent transactions
	            CONSTRAINT fk_tc_tx 
	                FOREIGN KEY (transaction_id) 
	                REFERENCES fullnode_transactions(id) 
	                DEFERRABLE INITIALLY DEFERRED,
	            CONSTRAINT fk_tc_prev_tx
	                FOREIGN KEY (previous_transaction_id)
	                REFERENCES fullnode_transactions(id)
	                DEFERRABLE INITIALLY DEFERRED,
	            -- role is SMALLINT but has no FK to token_role(id). Invalid role IDs can be inserted silently.
	            CONSTRAINT fk_tc_role 
	                FOREIGN KEY (role) 
	                REFERENCES token_role(id)
        );

        CREATE TABLE IF NOT EXISTS fullnode_tokenchain_index (
            token_id   TEXT PRIMARY KEY,
            index      INTEGER[] NOT NULL,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE TABLE IF NOT EXISTS fullnode_invalid_transactions (
        transaction JSON NOT NULL,
        reason TEXT NOT NULL,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
        );

        CREATE TABLE IF NOT EXISTS ipfs_providers (
            id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            cid            TEXT NOT NULL,
            peer_id        TEXT NOT NULL,
            did            TEXT,
            role           INTEGER,
            operation      TEXT NOT NULL,
            status         TEXT NOT NULL DEFAULT 'active',
            transaction_id TEXT,
            resource_type  TEXT,
            resource_id    TEXT,
            initiator      TEXT,
            owner          TEXT,
            token_value    NUMERIC,
            created_at     TIMESTAMPTZ DEFAULT NOW(),
            updated_at     TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_ipfs_providers_cid ON ipfs_providers(cid);
        CREATE INDEX IF NOT EXISTS idx_ipfs_providers_did ON ipfs_providers(did);
    `)
	return err
}
