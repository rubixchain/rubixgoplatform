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

// GetAllTransactions retrieves all transactions with optional limit and offset.
func (w *Wallet) GetAllTransactions() ([]models.Transactions, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at FROM transactions`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	defer rows.Close()

	var transactions []models.Transactions
	for rows.Next() {
		var tx models.Transactions
		err := rows.Scan(&tx.ID, &tx.Info, &tx.Signature, &tx.CreatedAt, &tx.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}
	return transactions, nil
}

// GetAllTransactions retrieves all transactions with optional limit and offset.
func (w *Wallet) GetAllTransactionsByOffset(limit, offset int) ([]models.Transactions, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, info, signature, created_at, updated_at
		 FROM transactions LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	defer rows.Close()

	var transactions []models.Transactions
	for rows.Next() {
		var tx models.Transactions
		err := rows.Scan(&tx.ID, &tx.Info, &tx.Signature, &tx.CreatedAt, &tx.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}
	return transactions, nil
}
