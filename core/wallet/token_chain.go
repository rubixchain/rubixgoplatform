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
// converts into bytes, and returns the chain of ordered transactions with a limit of 100 transactions,
// and the last transaction id of the array in order
func (w *Wallet) GetAllTransactionInfoByTokenId(tokenID string, txnId string) ([]models.Transactions, string, error) {
	txnChain := make([]models.Transactions, 0)
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
		txnChain = append(txnChain, *txn)
	}
	return txnChain, tokenChain[len(tokenChain)-1].TransactionID, nil
}

// GetTokenChainIndices retrieves the index array from tokenchain_index table for a given token.
// The index array represents the chronological order of tokenchain entries for this token.
func (w *Wallet) GetTokenChainIndices(tokenID string) ([]int, error) {
	var indices []int
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT index FROM tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&indices)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GetTokenChainIndices: no tokenchain_index found for token_id %s", tokenID)
		}
		return nil, fmt.Errorf("GetTokenChainIndices: failed to query tokenchain_index: %w", err)
	}

	return indices, nil
}

// GetTransactionsByTokenIDSingleQuery is an alternative single-query implementation.
// It performs the same operation as GetTransactionsByTokenID but in one database query.

func (w *Wallet) GetTransactionsByTokenID(tokenID string) ([]models.Transactions, error) {
	// Step 1: Get the index array from tokenchain_index
	var indices []int
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT index FROM tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&indices)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GetTransactionsByTokenID: no tokenchain_index found for token_id %s", tokenID)
		}
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to get indices: %w", err)
	}

	if len(indices) == 0 {
		return []models.Transactions{}, nil
	}

	// Step 2 & 3: Get transactions in exact order using unnest WITH ORDINALITY
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT t.id, t.info, t.signature, t.created_at, t.updated_at
		FROM unnest($1::int[]) WITH ORDINALITY AS idx(id, ord)
		JOIN tokenchain tc ON tc.id = idx.id
		JOIN transactions t ON t.id = tc.transaction_id
		ORDER BY idx.ord
	`, indices)
	if err != nil {
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to get transactions: %w", err)
	}

	transactions, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
	if err != nil {
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to collect rows: %w", err)
	}

	return transactions, nil
}

// GetSmartContractChainByTokenID retrieves all transactions for a smart contract token
// and converts them into TokenChainResponse format with TransactionID, Initiator, Epoch, and Data.
func (w *Wallet) GetSmartContractChainByTokenID(tokenID string) ([]models.TokenChainResponse, error) {
	// Step 1: Validate that the token is a smart contract
	isSC, err := w.IsSmartContract(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: %w", err)
	}
	if !isSC {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: token %s is not a smart contract", tokenID)
	}

	// Step 2: Get all transactions in chronological order
	transactions, err := w.GetTransactionsByTokenID(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: %w", err)
	}

	// Step 3: Convert transactions to TokenChainResponse using util function
	return util.ConvertToTokenChainResponses(transactions)
}

// GetNFTChainByTokenID retrieves all transactions for an NFT token
// and converts them into TokenChainResponse format with TransactionID, Initiator, Epoch, and Data.
func (w *Wallet) GetNFTChainByTokenID(tokenID string) ([]models.TokenChainResponse, error) {
	// Step 1: Validate that the token is an NFT
	isNFT, err := w.IsNFT(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetNFTChainByTokenID: %w", err)
	}
	if !isNFT {
		return nil, fmt.Errorf("GetNFTChainByTokenID: token %s is not an NFT", tokenID)
	}

	// Step 2: Get all transactions in chronological order
	transactions, err := w.GetTransactionsByTokenID(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetNFTChainByTokenID: %w", err)
	}

	// Step 3: Convert transactions to TokenChainResponse using util function
	return util.ConvertToTokenChainResponses(transactions)
}
