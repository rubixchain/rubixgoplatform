package parts

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func createGenesisTransaction(w *wallet.Wallet, dc types.DIDCrypto,
	freeTokens []models.Token, committedTokens []models.Token,
	did string, network string,
) (*models.TransactionInfo, *models.Signature, error) {
	var freeTokensInfo []*models.TokenInfo = make([]*models.TokenInfo, 0)
	var committedTokensInfo []*models.TokenInfo = make([]*models.TokenInfo, 0)

	for _, token := range freeTokens {
		prevTransactionID, err := w.ReturnLatestTransactionIdByTokenId(token.TokenID)
		if err != nil {
			return nil, nil, fmt.Errorf("createGenesisTransaction: failed to get latest transaction id for token %s, err: %v", token.TokenID, err)
		}

		freeTokensInfo = append(freeTokensInfo, &models.TokenInfo{
			TokenID:               token.TokenID,
			PreviousTransactionID: prevTransactionID,
		})
	}

	for _, token := range committedTokens {
		prevTransactionID, err := w.ReturnLatestTransactionIdByTokenId(token.TokenID)
		if err != nil {
			return nil, nil, fmt.Errorf("createGenesisTransaction: failed to get latest transaction id for token %s, err: %v", token.TokenID, err)
		}

		committedTokensInfo = append(committedTokensInfo, &models.TokenInfo{
			TokenID:               token.TokenID,
			PreviousTransactionID: prevTransactionID,
		})
	}

	txInfo := &models.TransactionInfo{
		Initiator: did,
		Owner:     did,
		Epoch:     int(util.GetCurrentTimeInUnix()),
		Network:   network,
		Tokens: &models.TransactionTokens{
			RBT: freeTokensInfo,
		},
		CommittedTokens: committedTokensInfo,
	}

	signatureBase64, err := util.SignTransaction(dc, txInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("createGenesisTransaction: failed to sign transaction, err: %v", err)
	}

	signature := &models.Signature{
		InitiatorSignature: signatureBase64,
	}

	return txInfo, signature, nil
}

func storeGenesisTx(w *wallet.Wallet, transaction models.Transactions) error {
	if err := w.CreateTransaction(&transaction); err != nil {
		return fmt.Errorf("storeGenesisTx: failed to store transaction info, err: %v", err)
	}

	return nil
}
