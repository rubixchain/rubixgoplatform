package wallet

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// CreateTransaction inserts a new transaction into the transactions table.
func (w *Wallet) CreateTransaction(tx *models.Transactions) error {
	_, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tx.ID, tx.Info, tx.Signature, time.Now(), time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}
	return nil
}

// CreateTransactionIfNotExists inserts a transaction if it does not already exist.
// Uses ON CONFLICT (id) DO NOTHING for idempotent sync inserts.
func (w *Wallet) CreateTransactionIfNotExists(tx *models.Transactions) error {
	_, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO NOTHING`,
		tx.ID, tx.Info, tx.Signature, time.Now(), time.Now(),
	)
	if err != nil {
		return fmt.Errorf("CreateTransactionIfNotExists: %w", err)
	}
	return nil
}

// GetTransactionByID retrieves a single transaction by its ID.
func (w *Wallet) GetTransactionByID(id string, isFullNode bool) (*models.Transactions, error) {
	var tx models.Transactions
	var err error
	if isFullNode {
		err = w.db.Pool().QueryRow(w.Ctx,
			`SELECT info, signature, created_at, updated_at
			 FROM fullnode_transactions WHERE id = $1`,
			id,
		).Scan(&tx.Info, &tx.Signature, &tx.CreatedAt, &tx.UpdatedAt)
	} else {
		err = w.db.Pool().QueryRow(w.Ctx,
			`SELECT id, info, signature, created_at, updated_at
			 FROM transactions WHERE id = $1`,
			id,
		).Scan(&tx.ID, &tx.Info, &tx.Signature, &tx.CreatedAt, &tx.UpdatedAt)
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transaction ID: %v is not present", id)
		}
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	return &tx, nil
}

// GetAllTransactions retrieves all transactions.
func (w *Wallet) GetAllTransactions() ([]models.Transactions, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at FROM transactions`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllTransactions: query: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
}

// GetAllTransactionsByOffset retrieves transactions with limit and offset.
func (w *Wallet) GetAllTransactionsByOffset(limit, offset int) ([]models.Transactions, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at
		 FROM transactions LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllTransactionsByOffset: query: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
}

// GetGenesisTransaction retrieves genesis transaction info for the given token id
func (w *Wallet) GetGenesisTransaction(tokenId string, isFullNode bool) (*models.Transactions, error) {
	genesisTxnId, err := w.GetGenesisTransactionIdByTokenId(tokenId, isFullNode)
	if err != nil {
		return nil, fmt.Errorf("GetGenesisTransaction: %v", err)
	}
	return w.GetTransactionByID(genesisTxnId, isFullNode)
}

// GetLatestTransaction retrieves latest transaction for the given token id
func (w *Wallet) GetLatestTransaction(tokenId string, isFullNode bool) (*models.Transactions, error) {
	latestTxnId, err := w.GetLatestTransactionIdByTokenId(tokenId, isFullNode)
	if err != nil {
		return nil, fmt.Errorf("GetLatestTransaction: %v", err)
	}
	return w.GetTransactionByID(latestTxnId, isFullNode)
}

// get latest transaction id of the given token id
func (w *Wallet) GetGenesisTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	var row pgx.Row
	var err error
	if isFullNode {
		row, err = w.db.Pool().Query(w.Ctx,
			`SELECT transaction_id 
     		FROM fullnode_tokenchain 
     		WHERE id = (
				SELECT index[1] 
				FROM fullnode_tokenchain_index 
				WHERE token_id = $1
			)`,
			tokenID,
		)
	} else {
		row, err = w.db.Pool().Query(w.Ctx,
			`SELECT transaction_id 
     		FROM tokenchain 
     		WHERE id = (
				SELECT index[1] 
				FROM tokenchain_index 
				WHERE token_id = $1
			)`,
			tokenID,
		)
	}
	if err != nil {
		return "", fmt.Errorf("GetGenesisTransactionIdByTokenId: %w", err)
	}
	var genesisTxnId string
	if err := row.Scan(&genesisTxnId); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("genesis transaction id not found for the token %s", tokenID)
		}
		return "", fmt.Errorf("GetTransactionIdByIndex scan: %w", err)
	}
	return genesisTxnId, nil
}

// ReturnLatestTransactionIdByTokenId is similar to GetLatestTransactionIdByTokenId
// It just returns empty string if no transaction is found for the given token id instead of returning an error
func (w *Wallet) ReturnLatestTransactionIdByTokenId(tokenID string) (string, error) {
	var row pgx.Row = w.db.Pool().QueryRow(w.Ctx,
		`SELECT transaction_id 
		FROM tokenchain 
		WHERE id = (
			SELECT index[array_upper(index, 1)] 
			FROM tokenchain_index 
			WHERE token_id = $1 
		)`,
		tokenID,
	)
	
	var latestTxnId string
	if err := row.Scan(&latestTxnId); err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("GetLatestTransactionIdByTokenId scan: %w", err)
	}
	return latestTxnId, nil
}


// get latest transaction id of the given token id
func (w *Wallet) GetLatestTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	var row pgx.Row
	if isFullNode {
		row = w.db.Pool().QueryRow(w.Ctx,
			`SELECT transaction_id 
			FROM fullnode_tokenchain 
			WHERE id = (
				SELECT index[array_upper(index, 1)] 
				FROM fullnode_tokenchain_index
				WHERE token_id = $1
			)`,
			tokenID,
		)
	} else {
		row = w.db.Pool().QueryRow(w.Ctx,
			`SELECT transaction_id 
			FROM tokenchain 
			WHERE id = (
				SELECT index[array_upper(index, 1)] 
				FROM tokenchain_index 
				WHERE token_id = $1 
			)`,
			tokenID,
		)
	}
	var latestTxnId string
	if err := row.Scan(&latestTxnId); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("latest transaction id not found for token %s", tokenID)
		}
		return "", fmt.Errorf("GetLatestTransactionIdByTokenId scan: %w", err)
	}
	return latestTxnId, nil
}

// get transaction id by Index id
func (w *Wallet) GetTransactionIdByIndex(index int16, isFullNode bool) (string, error) {
	var row pgx.Row
	if isFullNode {
		row = w.db.Pool().QueryRow(w.Ctx,
			`SELECT transaction_id FROM fullnode_tokenchain WHERE id = $1`,
			index,
		)
	} else {
		row = w.db.Pool().QueryRow(w.Ctx,
			`SELECT transaction_id FROM tokenchain WHERE id = $1`,
			index,
		)
	}
	var txnId string
	if err := row.Scan(&txnId); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("transaction id not found for the index %d", index)
		}
		return "", fmt.Errorf("GetTransactionIdByIndex scan: %w", err)
	}
	return txnId, nil
}

// GetTransactionsByDIDAndTokenType returns transaction history of a given DID and token_type, 
// and in case of RBT it does not return split/burnt transactions where initiator=owner
func (w *Wallet) GetTransactionsByDIDAndTokenType(did, tokenType string) ([]models.Transactions, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at
	FROM transactions
	WHERE (info->>'initiator' = $1 OR info->>'owner' = $1)
	AND (
		CASE $2
			WHEN 'rbt' THEN
				json_typeof(info->'tokens'->'rbt') = 'array'
				AND info->>'initiator' != info->>'owner'

			WHEN 'nft' THEN
				json_typeof(info->'tokens'->'nft') = 'array'

			WHEN 'ft' THEN
				json_typeof(info->'tokens'->'ft') = 'array'

			WHEN 'smartContract' THEN
				json_typeof(info->'tokens'->'smartContract') = 'array'

			ELSE FALSE
		END
	)`,
		did, tokenType,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTransactionsByAddressAndTokenType: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
}
