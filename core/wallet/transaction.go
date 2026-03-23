package wallet

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// CreateTransaction inserts a new transaction into the transactions table.
// BYPASS: do not use for genesis token minting — this path writes transactions only,
// has no ON CONFLICT clause (will error on duplicate id), and skips tokens, tokenchain,
// tokenchain_index, and transaction_units. Use PersistGenesisTokenRecord instead.
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

// GetTransactionByID retrieves a single transaction by its ID.
func (w *Wallet) GetTransactionByID(id string) (*models.Transactions, error) {
	var tx models.Transactions
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at
		 FROM transactions WHERE id = $1`,
		id,
	).Scan(&tx.ID, &tx.Info, &tx.Signature, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
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
	return w.GetTransactionByID(genesisTxnId)
}

// GetLatestTransaction retrieves genesis transaction info for the given token id
func (w *Wallet) GetLatestTransaction(tokenId string, isFullNode bool) (*models.Transactions, error) {
	latestTxnId, err := w.GetLatestTransactionIdByTokenId(tokenId, isFullNode)
	if err != nil {

	}
	return w.GetTransactionByID(latestTxnId)
}

// get latest transaction id of the given token id
func (w *Wallet) GetGenesisTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	var tableName string
	if isFullNode {
		tableName = constants.FullnodeTokenchainIndexTable
	} else {
		tableName = constants.TokenchainIndexTable
	}
	query := fmt.Sprintf(`SELECT index[1] FROM %s WHERE token_id = $1 AND previous_transaction_id = ''`, pgx.Identifier{tableName}.Sanitize())

	row, err := w.db.Pool().Query(w.Ctx, query, tokenID)
	if err != nil {
		return "", fmt.Errorf("GetGenesisTransactionIdByTokenId: %w", err)
	}
	var genesisTxnIdIndex int
	if err := row.Scan(&genesisTxnIdIndex); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("latest transaction id not found for token %s", tokenID)
		}
		return "", fmt.Errorf("GetLatestTransactionIdByTokenId scan: %w", err)
	}

	return w.GetTransactionIdByIndex(int16(genesisTxnIdIndex), isFullNode)
}

// get latest transaction id of the given token id
func (w *Wallet) GetLatestTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	var tableName string
	if isFullNode {
		tableName = constants.FullnodeTokenchainIndexTable
	} else {
		tableName = constants.TokenchainIndexTable
	}
	query := fmt.Sprintf(`SELECT index[array_upper(index, 1)] FROM %s WHERE token_id = $1`, pgx.Identifier{tableName}.Sanitize())

	row, err := w.db.Pool().Query(w.Ctx, query, tokenID)
	if err != nil {
		return "", fmt.Errorf("GetLatestTransactionIdByTokenId: %w", err)
	}
	var latestTxnIdIndex int
	if err := row.Scan(&latestTxnIdIndex); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("latest transaction id not found for token %s", tokenID)
		}
		return "", fmt.Errorf("GetLatestTransactionIdByTokenId scan: %w", err)
	}

	return w.GetTransactionIdByIndex(int16(latestTxnIdIndex), isFullNode)
}

// get transaction id by Index id
func (w *Wallet) GetTransactionIdByIndex(index int16, isFullNode bool) (string, error) {
	var tableName string
	if isFullNode {
		tableName = constants.FullnodeTokenchainTable
	} else {
		tableName = constants.TokenchainTable
	}
	query := fmt.Sprintf(`SELECT transaction_id FROM %s WHERE id = $1`, pgx.Identifier{tableName}.Sanitize())
	row, err := w.db.Pool().Query(w.Ctx, query, index)
	if err != nil {
		return "", fmt.Errorf("GetTransactionIdByIndex: %w", err)
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
