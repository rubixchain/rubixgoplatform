package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (w *Wallet) GetTokenChainByTokenID(tokenID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, transaction_id, role, position, created_at, updated_at
		 FROM tokenchain WHERE token_id = $1 ORDER BY position`, tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenID: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// GetTokenChainByTokenIDAndPrevTxnId fetches the token chain from the input transaction id, with a limit of 100 transactions
func (w *Wallet) GetTokenChainByTokenIDAndPrevTxnId(tokenID string, txnID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT tc.*
			FROM tokenchain tc
			WHERE tc.token_id = $1
			AND tc.position >= (
				SELECT position
				FROM tokenchain
				WHERE token_id = $1
				AND prev_txn_id = $2
				)
			ORDER BY tc.position
			LIMIT 100`, tokenID, txnID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenIDAndPrevTxnId: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// get latest transaction id of the given token id
func (w *Wallet) GetGenesisTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	var tableName string
	if isFullNode {
		tableName = "fullnode_tokenchain_index"
	} else {
		tableName = "tokenchain_index"
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
		tableName = "fullnode_tokenchain_index"
	} else {
		tableName = "tokenchain_index"
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
		tableName = "fullnode_tokenchain"
	} else {
		tableName = "tokenchain"
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

// get token role in latest transaction of the given token id
func (w *Wallet) GetRoleOfTokenIdInLatestTxn(tokenID string, isFullNode bool) (int16, error) {
	var tableName string
	if isFullNode {
		tableName = "fullnode_tokenchain"
	} else {
		tableName = "tokenchain"
	}
	query := fmt.Sprintf(`SELECT role FROM %s WHERE token_id = $1`, pgx.Identifier{tableName}.Sanitize())
	row, err := w.db.Pool().Query(w.Ctx, query, tokenID)
	if err != nil {
		return -1, fmt.Errorf("GetRoleOfTokenIdInLatestTxn: %w", err)
	}
	var role int16
	if err := row.Scan(&role); err != nil {
		if err == pgx.ErrNoRows {
			return -1, fmt.Errorf("latest token role not found for token %s", tokenID)
		}
		return -1, fmt.Errorf("GetRoleOfTokenIdInLatestTxn scan: %w", err)
	}
	return role, nil
}

func (w *Wallet) AddTokenChainEntry(entry *models.TokenChain) error {
	_, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO tokenchain (token_id, transaction_id, role, position, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, entry.TokenID, entry.TransactionID, entry.Role, entry.Position)
	if err != nil {
		return fmt.Errorf("AddTokenChainEntry: %w", err)
	}
	return nil
}

func (w *Wallet) GetTransactionAndRoleAtHeight(tokenID string, position int64) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx, `
		SELECT transaction_id,role FROM tokenchain WHERE token_id = $1 AND position = $2 
		ORDER BY created_at DESC LIMIT 1`, tokenID, position,
	)
	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, fmt.Errorf("transaction not found at position %d for token %s", position, tokenID)
		}
		return nil, -1, fmt.Errorf("GetTransactionAtHeight scan: %w", err)
	}

	tx, err := w.GetTransactionByID(txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetTransactionAtHeight transaction details not found for transaction_id: %v, err %w", txID, err)
	}

	return tx, tokenRoleInTx, nil
}

// GetAllTransactionInfoInBytesByTokenId fetches entire token chain, fetches each transaction by transactionId and
// converts into bytes, and returns the chain of ordered transactions in byte array with a limit of 100 transactions,
// and the last transaction id of the array in order
func (w *Wallet) GetAllTransactionInfoInBytesByTokenId(tokenID string, txnId string) ([][]byte, string, error) {
	txnChain := make([][]byte, 0)
	// get token chain of the token with height limit of 100
	tokenChain, err := w.GetTokenChainByTokenIDAndPrevTxnId(tokenID, txnId)
	if err != nil {
		return nil, "", fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get token chain; error: %v ", err)
	}

	// process each txn in the chain in a loop
	for _, txnInfo := range tokenChain {
		// fetch the txn by txnId
		txn, err := w.GetTransactionByID(txnInfo.TransactionID)
		if err != nil {
			return nil, "", fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get transaction by id; error: %v ", err)
		}

		// convert txn to bytes
		txnBytes, err := util.TransactionToBytes(txn)
		if err != nil {
			return nil, "", fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to convert transaction into bytes; error: %v ", err)
		}
		txnChain = append(txnChain, txnBytes)
	}
	return txnChain, tokenChain[len(tokenChain)-1].TransactionID, nil
}
