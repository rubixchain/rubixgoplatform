package consensus

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// The input can be made into a struct. Right now added all inputs separately to get a clear picture
func ReqPledgeToken(
	dc types.DIDCrypto,
	w *wallet.Wallet,
	transactionValue float64,
	networkMode string,
	log logger.Logger,
	pubsub *types.PubSub,
	referenceId string,
) (models.PledgeTokenResponse, error) {

	pledgeTokenDetails, _, err := parts.CollectRBTTokens(
		dc,
		w,
		transactionValue,
		networkMode,
		log,
		pubsub,
	)

	if err != nil {
		log.Error("Failed to get tokens", "err", err)
		return models.PledgeTokenResponse{}, err
	}

	if len(pledgeTokenDetails) == 0 {
		return models.PledgeTokenResponse{}, fmt.Errorf("no tokens left to pledge")
	}

	pledgeResponse := models.PledgeTokenResponse{
		ReferenceId:  referenceId,
		PledgeTokens: pledgeTokenDetails,
	}

	return pledgeResponse, nil
}
func InitiateConsensus(consensusRequest models.ConsensusRequest, quorumDc types.DIDCrypto, w *wallet.Wallet, log logger.Logger) (*models.ConsensusResponse, error) {
	quorumDid := quorumDc.GetDID()
	isTransactionInfoValidatded, err := ValidateTransaction()
	if err != nil {
		log.Error("InitiateConsensus : Failed to validate transaction info", "err", err)

		return &models.ConsensusResponse{}, err
	}

	if !isTransactionInfoValidatded {
		log.Error("InitiateConsensus : Transaction info validation failed")
		return &models.ConsensusResponse{}, fmt.Errorf("transaction info validation failed")
	}
	// This is the pledgeTokenInformation we need to pass to the PledgeTokens function.
	// This needs to be convered to []Tstring basically the list of token ids which are pledged for this transaction.
	pledgeDetails := consensusRequest.TransactionInfo.Quorums
	// Here we are assuming there is only one quorum
	// This will change when multple quorums are involved
	if len(pledgeDetails) == 0 {
		log.Error("InitiateConsensus : No pledge details found")
		return &models.ConsensusResponse{}, fmt.Errorf("no pledge details found")
	}
	pledgeTokenDetails := pledgeDetails[0].Tokens
	// pledgeTokenList := util.ExtractTokenIDs(pledgeTokenDetails) // Need to ensure this function logic is not existing at th emoment

	quorumSignature, err := util.SignTransaction(quorumDc, consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus : Failed to sign transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}
	transactionId, err := util.GetTransactionID(consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus : Failed to get transaction ID", "err", err)
		return &models.ConsensusResponse{}, err
	}
	transactionInfoBytes, err := json.Marshal(consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus : Failed to marshal transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}
	quorumSignatureInfo := models.QuorumSignature{
		Did:       quorumDid,
		Signature: quorumSignature,
	}

	signature := models.Signature{
		InitiatorSignature: consensusRequest.InitiatorSignature,
		Quorums:            []models.QuorumSignature{quorumSignatureInfo},
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		log.Error("InitiateConsensus : Failed to marshal signature", "err", err)
		return &models.ConsensusResponse{}, err
	}

	transactions := &models.Transactions{
		ID:        transactionId,
		Info:      json.RawMessage(transactionInfoBytes),
		Signature: json.RawMessage(signatureBytes),
	}

	consensusResponse := models.ConsensusResponse{
		ReferenceId:     consensusRequest.ReferenceId,
		QuorumSignature: quorumSignature,
		Message:         "Transaction Information verified succesfully. Consensus Complete.",
		Status:          true,
	}
	//List of Pledge token ids
	// The incoming TransactionInfo with the signature of both initiator and quorumSignature
	err = w.PledgeTokens(pledgeTokenDetails, transactions, quorumDid, int64(consensusRequest.TransactionInfo.Epoch))
	if err != nil {
		log.Error("InitiateConsensus : Failed to pledge tokens", "err", err)
		return &models.ConsensusResponse{}, err
	}

	return &consensusResponse, nil
}
