package wallet

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) ListFTs() ([]*models.FT, error) {
	rows, err := w.db.Pool().Query(
		w.Ctx, "SELECT ft_name, creator_did, ft_count FROM fts",
	)
	if err != nil {
		return nil, err
	}

	var ftList []*models.FT = make([]*models.FT, 0)
	for rows.Next() {
		var ftInfo *models.FT = &models.FT{}
		if err := rows.Scan(&ftInfo.FTName, &ftInfo.CreatorDID, &ftInfo.FTCount); err != nil {
			return nil, err
		}

		ftList = append(ftList, ftInfo)
	}

	return ftList, nil
}
