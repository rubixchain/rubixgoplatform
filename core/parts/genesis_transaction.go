package parts

import (
	"encoding/hex"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func createGenesisTransaction(dc types.DIDCrypto,
	freeTokens []models.Token, committedTokens []models.Token,
	did string, network string,
) (*models.TransactionInfo, *models.Signature, error) {
	var freeTokensInfo []*models.TokenInfo = make([]*models.TokenInfo, 0)
	var committedTokensInfo []*models.TokenInfo = make([]*models.TokenInfo, 0)

	for _, token := range freeTokens {
		freeTokensInfo = append(freeTokensInfo, &models.TokenInfo{
			TokenID:               token.TokenID,
			PreviousTransactionID: token.TransactionID,
		})
	}

	for _, token := range committedTokens {
		committedTokensInfo = append(committedTokensInfo, &models.TokenInfo{
			TokenID:               token.TokenID,
			PreviousTransactionID: token.TransactionID,
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

	txInfoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("createGenesisTransaction: failed to serialize transaction info: %w", err)
	}

	signatureBytes, err := dc.PvtSign(txInfoBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("createGenesisTransaction: failed to sign transaction, err: %v", err)
	}

	signature := &models.Signature{
		InitiatorSignature: hex.EncodeToString(signatureBytes),
	}

	return txInfo, signature, nil
}

// storeGenesisTx calls CreateTransaction directly.
// BYPASS: do not use — this path writes the transactions table only and skips
// tokens, tokenchain, tokenchain_index, and transaction_units.
// Callers in CollectRBTTokens use this path; routing through PersistGenesisTokenRecord
// is deferred as an OPEN QUESTION for the next phase (see ANALYSIS.md).
func storeGenesisTx(w *wallet.Wallet, transaction models.Transactions) error {
	if err := w.CreateTransaction(&transaction); err != nil {
		return fmt.Errorf("storeGenesisTx: failed to store transaction info, err: %v", err)
	}

	return nil
}
