package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (w *Wallet) GetTokenChainByTokenID(tokenID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, transaction_id, role, height, created_at, updated_at
		 FROM tokenchain WHERE token_id = $1 ORDER BY height`, tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenID: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// get latest transaction id of the given token id
func (w *Wallet) GetGenesisTransactionIdByTokenId(tokenID string) (string, error) {
	row, err := w.db.Pool().Query(w.Ctx,
		`SELECT index[1]
			FROM tokenchain_index
			WHERE token_id = $1`, tokenID,
	)
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

	return w.GetTransactionIdByIndex(int16(genesisTxnIdIndex))
}

// get latest transaction id of the given token id
func (w *Wallet) GetLatestTransactionIdByTokenId(tokenID string) (string, error) {
	row, err := w.db.Pool().Query(w.Ctx,
		`SELECT index[array_upper(index, 1)]
			FROM tokenchain_index
			WHERE token_id = $1`, tokenID,
	)
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

	return w.GetTransactionIdByIndex(int16(latestTxnIdIndex))
}

// get transaction id by Index id
func (w *Wallet) GetTransactionIdByIndex(index int16) (string, error) {
	row, err := w.db.Pool().Query(w.Ctx,
		`SELECT transaction_id
		FROM tokenchain
		WHERE id = $1`, index,
	)
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
func (w *Wallet) GetRoleOfTokenIdInLatestTxn(tokenID string) (int16, error) {
	row, err := w.db.Pool().Query(w.Ctx,
		`SELECT role
		FROM tokenchain
		WHERE token_id = $1`, tokenID,
	)
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

// func (w *Wallet) GetLatestTransactionAndRoleByTokenID(tokenID string) (*models.Transactions, int16, error) {
// 	row := w.db.Pool().QueryRow(w.Ctx,
// 		`SELECT transaction_id, role FROM tokenchain WHERE token_id = $1 ORDER BY height DESC LIMIT 1`, tokenID,
// 	)

// 	var txID string
// 	var tokenRoleInTx int16
// 	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
// 		if err == pgx.ErrNoRows {
// 			return nil, -1, nil
// 		}
// 		return nil, -1, fmt.Errorf("GetLatestTransactionByTokenID scan: %w", err)
// 	}

// 	tx, err := w.GetTransactionByID(txID)
// 	if err != nil {
// 		return nil, -1, fmt.Errorf("GetLatestTransactionByTokenID GetTransactionByID: %w", err)
// 	}

// 	return tx, tokenRoleInTx, nil
// }

func (w *Wallet) AddTokenChainEntry(entry *models.TokenChain) error {
	_, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO tokenchain (token_id, transaction_id, role, height, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, entry.TokenID, entry.TransactionID, entry.Role, entry.Height)
	if err != nil {
		return fmt.Errorf("AddTokenChainEntry: %w", err)
	}
	return nil
}

func (w *Wallet) GetTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx, `
		SELECT transaction_id,role FROM tokenchain WHERE token_id = $1 AND height = $2 
		ORDER BY created_at DESC LIMIT 1`, tokenID, height,
	)
	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, fmt.Errorf("transaction not found at height %d for token %s", height, tokenID)
		}
		return nil, -1, fmt.Errorf("GetTransactionAtHeight scan: %w", err)
	}

	tx, err := w.GetTransactionByID(txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetTransactionAtHeight transaction details not found for transaction_id: %v, err %w", txID, err)
	}

	return tx, tokenRoleInTx, nil
}

// GetAllTransactionsInBytesByTokenId fetches entire token chain, fetches each transaction by transactionId and 
// converts into bytes, and returns the chain of transactions in byte array
func (w *Wallet) GetAllTransactionsInBytesByTokenId(tokenID string) ([][]byte, error) {
	txnChain := make([][]byte, 0)
	// get entire token chain of the token
	tokenChain, err := w.GetTokenChainByTokenID(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get token chain; error: %v ", err)
	}

	// process each txn in the chain in a loop
	for _, txnInfo := range tokenChain {
		// fetch the txn by txnId
		txn, err := w.GetTransactionByID(txnInfo.TransactionID)
		if err != nil {
			return nil, fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get transaction by id; error: %v ", err)
		}

		// convert txn to bytes
		txnBytes, err := util.TransactionToBytes(txn)
		if err != nil {
			return nil, fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to convert transaction into bytes; error: %v ", err)
		}
		txnChain = append(txnChain, txnBytes)
	}
	return txnChain, nil
}
