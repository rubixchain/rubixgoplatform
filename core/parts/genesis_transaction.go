package parts

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func createGenesisTransaction(dc did.DIDCrypto,
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

	txInfoBytes, err := json.Marshal(txInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("createGenesisTransaction: failed to marshal transaction info, err: %v", err)
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

func publishTransaction(pubsub types.PubSub, tx *models.TransactionInfo, signature *models.Signature) (*models.Transactions, error) {
	txID, err := util.GetTransactionID(tx)
	if err != nil {
		return nil, fmt.Errorf("publishTransaction: failed to get transaction ID: %v", err)
	}

	txInfoBytes, err := json.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("publishTransaction: failed to marshal transactionInfo, err: %v", err)
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		return nil, fmt.Errorf("publishTransaction: failed to marshal Signature, err: %v", err)
	}

	transaction := &models.Transactions{
		ID:        txID,
		Info:      txInfoBytes,
		Signature: signatureBytes,
	}

	eventTx := models.EventTransaction{
		Status:      true,
		Transaction: transaction,
	}

	if err := pubsub.Publish(constants.Event_RubixTxns, eventTx); err != nil {
		return nil, fmt.Errorf("publishTransaction: failed to publish genesis transaction, err: %v", err)
	}

	return transaction, nil
}

func storeGenesisTx(w *wallet.Wallet, transaction models.Transactions) error {
	if err := w.CreateTransaction(&transaction); err != nil {
		return fmt.Errorf("storeGenesisTx: failed to store transaction info, err: %v", err)
	}

	return nil
}
