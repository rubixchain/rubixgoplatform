package wallet

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) ListFTs() ([]*models.FT, error) {
	rows, err := w.db.Pool().Query(
		w.Ctx, "SELECT id, ft_name, ft_count, creator_did FROM fts",
	)
	if err != nil {
		return nil, err
	}

	var ftList []*models.FT = make([]*models.FT, 0)
	for rows.Next() {
		var ftInfo *models.FT = &models.FT{}
		if err := rows.Scan(&ftInfo.ID, &ftInfo.FTName, &ftInfo.FTCount, &ftInfo.CreatorDID); err != nil {
			return nil, err
		}

		ftList = append(ftList, ftInfo)
	}

	return ftList, nil
}
