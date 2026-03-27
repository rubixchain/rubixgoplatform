package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) AddQuorum(quorumAddressInfo models.QuorumManager) error {
	rows, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO quorum_manager(did) 
		VALUES ($1)
		ON CONFLICT(did) DO NOTHING;
	`, quorumAddressInfo.Did)
	if err != nil {
		return fmt.Errorf("failed to add quorum info for did: %v, err: %v", quorumAddressInfo.Did, err)
	}

	if rows.RowsAffected() == 0 {
		return fmt.Errorf("quorum did %v already exists", quorumAddressInfo.Did)
	}

	return nil
}

func (w *Wallet) RemoveQuorum(quorumAddressInfo models.QuorumManager) error {
	rows, err := w.db.Pool().Exec(w.Ctx, `DELETE FROM quorum_manager WHERE did=$1;`, quorumAddressInfo.Did)
	if err != nil {
		return err
	}

	if rows.RowsAffected() == 0 {
		return fmt.Errorf("quorum address %v already exists", quorumAddressInfo.Did)
	}

	return nil
}

func (w *Wallet) RemoveAllQuorums() error {
	if _, err := w.db.Pool().Exec(w.Ctx, `TRUNCATE TABLE quorum_manager;`); err != nil {
		return fmt.Errorf("unable to remove all records from quorum_manager table, err: %v", err)
	}

	return nil
}

func (w *Wallet) IsQuorumExist(did string) (bool, error) {
	var exists bool

	err := w.db.Pool().QueryRow(
		w.Ctx,
		`SELECT EXISTS(
			SELECT 1 FROM quorum_manager WHERE did=$1
		)`,
		did,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("IsQuorumExist: query failed: %w", err)
	}

	return exists, nil
}

func (w *Wallet) GetAllQuorums() ([]models.QuorumManager, error) {
	rows, err := w.db.Pool().Query(
		w.Ctx,
		`SELECT did FROM quorum_manager`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllQuorums: query failed: %w", err)
	}
	defer rows.Close()

	var quorums []models.QuorumManager

	for rows.Next() {
		var q models.QuorumManager

		if err := rows.Scan(&q.Did); err != nil {
			return nil, fmt.Errorf("GetAllQuorums: scan failed: %w", err)
		}

		quorums = append(quorums, q)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetAllQuorums: rows iteration error: %w", err)
	}

	return quorums, nil
}