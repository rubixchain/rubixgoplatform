-- Migration: 001_drop_block_hash_columns.sql
--
-- Purpose:
--   Drop the block_hash column from legacy GORM-managed tables that may still have
--   it from older schema versions. BlockHash was always set to the transaction ID in
--   the pubsub/fullnode paths and is a vestige of the removed block package.
--
-- Scope:
--   This migration targets tables created by the old GORM auto-migrate path:
--     - pub_sub_txn_info
--     - full_node_txn_history_info
--
--   The current codebase uses pgxpool with core/storage/schema.go; those tables
--   (fullnode_rbt, etc.) never had a block_hash column and are not affected.
--
-- Idempotent:
--   Safe to run multiple times. All DDL statements use IF EXISTS guards.
--   The pre-check DO block only runs if the table exists.
--
-- How to run:
--   psql -h <host> -U <user> -d <database> -f 001_drop_block_hash_columns.sql
--   Run manually against any PostgreSQL database created by the old GORM wrapper path.

-- Pre-check: transaction_id must be NOT NULL and UNIQUE before promoting to PK.
-- If this check fails, abort the entire migration and report violating rows.
DO $$
DECLARE
  _bad_count INTEGER;
BEGIN
  -- Only run check if the table exists (legacy GORM-created table)
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'pub_sub_txn_info') THEN
    SELECT COUNT(*) INTO _bad_count
      FROM (
        SELECT transaction_id
          FROM pub_sub_txn_info
         GROUP BY transaction_id
        HAVING COUNT(*) > 1 OR transaction_id IS NULL
      ) sub;

    IF _bad_count > 0 THEN
      RAISE EXCEPTION 'MIGRATION ABORTED: pub_sub_txn_info has % transaction_id group(s) that are NULL or duplicated. Fix data before migrating.', _bad_count;
    END IF;
  END IF;
END $$;

-- Drop block_hash from pub_sub_txn_info
ALTER TABLE IF EXISTS pub_sub_txn_info DROP COLUMN IF EXISTS block_hash;

-- Drop block_hash from full_node_txn_history_info
ALTER TABLE IF EXISTS full_node_txn_history_info DROP COLUMN IF EXISTS block_hash;
