package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) CreateOrUpdateDID(didInfo *models.DID) error {
	if _, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO dids(did, peer_id, local, algo_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(did)
		DO UPDATE SET
			peer_id = EXCLUDED.peer_id,
			local = EXCLUDED.local,
			algo_id = EXCLUDED.algo_id;
	`, didInfo.DID, didInfo.PeerID, didInfo.Local, didInfo.AlgoID); err != nil {
		return fmt.Errorf("CreateOrUpdateDID: failed to create or update DID %v information, err: %v", didInfo.DID, err)
	}

	return nil
}

func (w *Wallet) GetDID(did string) (models.DID, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT did, peer_id, local, algo_id FROM dids WHERE did=$1`, did,
	)

	var didInfo models.DID
	if err := row.Scan(&didInfo.DID, &didInfo.PeerID, &didInfo.Local, &didInfo.AlgoID); err != nil {
		if err == pgx.ErrNoRows {
			return models.DID{}, fmt.Errorf("GetDID: no record for DID %v found", did)
		}

		return models.DID{}, fmt.Errorf("GetDID: failed to fetch DID: %v, err: %v", did, err)
	}

	return didInfo, nil
}
func (w *Wallet) GetPeerID(did string) (string, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT peer_id FROM dids WHERE did=$1`, did,
	)
	
	var peerID string
	if err := row.Scan(&peerID); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("GetPeerID: no record for PeerID of DID %v found", did)
		}

		return "", fmt.Errorf("GetPeerID: failed to fetch DID: %v, err: %v", did, err)
	}

	return peerID, nil
}

func (w *Wallet) GetAllDID() ([]string, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT did FROM dids`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllDID: failed to query dids table, err: %v", err)
	}
	defer rows.Close()

	dids := make([]string, 0)
	for rows.Next() {
		var did string
		err := rows.Scan(&did)
		if err != nil {
			return nil, fmt.Errorf("GetAllDID: error while scanning rows, err: %v", err)
		}

		dids = append(dids, did)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return dids, nil
}

func (w *Wallet) IsDIDExists(did string) (bool, error) {
	var exists bool

	err := w.db.Pool().QueryRow(
		w.Ctx,
		`SELECT EXISTS(
			SELECT 1 FROM dids WHERE did = $1
		)`,
		did,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("IsDIDExists: query failed: %w", err)
	}

	return exists, nil
}