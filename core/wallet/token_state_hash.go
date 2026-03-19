package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) RemoveTokenStateHashes(transactionIds []string) error {
	if len(transactionIds) == 0 {
		return nil
	}
	
	if _, err := w.db.Pool().Exec(w.Ctx,
		`DELETE FROM token_state_hashes WHERE transaction_id=ANY($1)`,
		transactionIds,
	); err != nil {
		return fmt.Errorf("RemoveTokenStateHashes: failed to delete records from token_state_hashes table, err: %v", err)
	}

	return nil
}

func (w *Wallet) GetTokenStateHashes(transactionId string) ([]models.TokenStateHash, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT did, token_state_hash, pledged_token, transaction_id FROM token_state_hashes
		`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokenStateHashes, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenStateHash])
	if err != nil {
		return nil, fmt.Errorf("GetTokenStateHashes: failed to collect rows, err: %v", err)
	}

	return tokenStateHashes, nil
}

func (w *Wallet) AddTokenStateHashes(hashes []models.TokenStateHash) error {
    if len(hashes) == 0 {
        return nil
    }

    batch := &pgx.Batch{}

    for _, h := range hashes {
        batch.Queue(
            `INSERT INTO token_state_hashes 
            (did, token_state_hash, pledged_token, transaction_id)
            VALUES ($1, $2, $3, $4)`,
            h.DID,
            h.TokenStateHash,
            h.PledgedToken,
            h.TransactionID,
        )
    }

    br := w.db.Pool().SendBatch(w.Ctx, batch)
    defer br.Close()

    for range hashes {
        if _, err := br.Exec(); err != nil {
            return fmt.Errorf("AddTokenStateHashes: insert failed: %w", err)
        }
    }

    return nil
}
