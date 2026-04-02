package consensus

import (
	"context"
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

	log.Info("ReqPledgeToken : Request received to pledge tokens for transaction", "referenceId", referenceId, "transactionValue", transactionValue, "networkMode", networkMode)
	// Lock and fetch free RBT tokens for split/transfer.
	lockedTokens, err := w.LockTokensForSplit(context.Background(), dc.GetDID(), transactionValue)
	if err != nil {
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to lock tokens for split: %w", err)
	}
	denomMap, err := w.GetTokenDenomArray(dc.GetDID())
	if err != nil {
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to fetch token denom array: %w", err)
	}

	pledgeTokenDetails, _, _, _, err := parts.CollectRBTTokens(
		dc,
		w,
		transactionValue,
		lockedTokens,
		denomMap,
		networkMode,
		log,
	)
	// TODO(phase09): handle childRecords and parentsToBurn for pledge path

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

	// Unmarshal Transaction.Info to get TransactionInfo
	var txnInfo models.TransactionInfo
	if err := json.Unmarshal(consensusRequest.Transaction.Info, &txnInfo); err != nil {
		log.Error("InitiateConsensus: failed to unmarshal transaction info", "err", err)
		return &models.ConsensusResponse{}, fmt.Errorf("InitiateConsensus: failed to unmarshal transaction info: %w", err)
	}

	// Unmarshal Transaction.Signature to get the initiator signature
	var incomingSig models.Signature
	if err := json.Unmarshal(consensusRequest.Transaction.Signature, &incomingSig); err != nil {
		log.Error("InitiateConsensus: failed to unmarshal signature", "err", err)
		return &models.ConsensusResponse{}, fmt.Errorf("InitiateConsensus: failed to unmarshal signature: %w", err)
	}

	// This is the pledgeTokenInformation we need to pass to the PledgeTokens function.
	// This needs to be convered to []Tstring basically the list of token ids which are pledged for this transaction.
	pledgeDetails := txnInfo.Quorums
	// Here we are assuming there is only one quorum
	// This will change when multple quorums are involved
	if len(pledgeDetails) == 0 {
		log.Error("InitiateConsensus : No pledge details found")
		return &models.ConsensusResponse{}, fmt.Errorf("no pledge details found")
	}
	pledgeTokenDetails := pledgeDetails[0].Tokens

	quorumSignature, err := util.SignTransaction(quorumDc, &txnInfo)
	if err != nil {
		log.Error("InitiateConsensus : Failed to sign transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}

	// Use Transaction.ID directly — no recomputation needed
	transactionId := consensusRequest.Transaction.ID

	// Use Transaction.Info bytes directly — no re-serialization needed
	transactionInfoBytes := consensusRequest.Transaction.Info

	quorumSignatureInfo := models.QuorumSignature{
		Did:       quorumDid,
		Signature: quorumSignature,
	}

	signature := models.Signature{
		InitiatorSignature: incomingSig.InitiatorSignature,
		Quorums:            []models.QuorumSignature{quorumSignatureInfo},
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		log.Error("InitiateConsensus : Failed to marshal signature", "err", err)
		return &models.ConsensusResponse{}, err
	}

	transactions := &models.Transactions{
		ID:        transactionId,
		Info:      transactionInfoBytes,
		Signature: signatureBytes,
	}

	consensusResponse := models.ConsensusResponse{
		ReferenceId:     consensusRequest.ReferenceId,
		QuorumSignature: quorumSignature,
		Message:         "Transaction Information verified succesfully. Consensus Complete.",
		Status:          true,
	}
	//List of Pledge token ids
	// The incoming TransactionInfo with the signature of both initiator and quorumSignature
	err = w.PledgeTokens(pledgeTokenDetails, transactions, quorumDid, int64(txnInfo.Epoch))
	if err != nil {
		log.Error("InitiateConsensus : Failed to pledge tokens", "err", err)
		return &models.ConsensusResponse{}, err
	}

	return &consensusResponse, nil
}
