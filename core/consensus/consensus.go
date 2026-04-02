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

	// Lock and fetch free RBT tokens for split/transfer.
	log.Debug("ReqPledgeToken: Attempting to lock tokens", "did", dc.GetDID(), "amount", transactionValue)
	lockedTokens, err := w.LockTokensForSplit(context.Background(), dc.GetDID(), transactionValue)
	if err != nil {
		log.Error("ReqPledgeToken: Failed to lock tokens", "err", err)
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to lock tokens for split: %w", err)
	}
	log.Info("ReqPledgeToken: Tokens locked", "lockedTokensCount", len(lockedTokens), "transactionValue", transactionValue)
	fmt.Println("The transactionValue in ReqPledgeToken is ", transactionValue)

	denomMap, err := w.GetTokenDenomArray(dc.GetDID())
	if err != nil {
		log.Error("ReqPledgeToken: Failed to get denom array", "err", err)
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to fetch token denom array: %w", err)
	}
	log.Debug("ReqPledgeToken: Denom map retrieved", "denomMapSize", len(denomMap))

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
		log.Error("ReqPledgeToken: CollectRBTTokens failed", "err", err)
		return models.PledgeTokenResponse{}, err
	}

	log.Info("ReqPledgeToken: CollectRBTTokens completed", "pledgeTokenCount", len(pledgeTokenDetails))
	for i, token := range pledgeTokenDetails {
		log.Debug("ReqPledgeToken: Pledge token detail", "index", i, "tokenID", token.TokenID, "prevTxID", token.PreviousTransactionID)
	}

	if len(pledgeTokenDetails) == 0 {
		log.Error("ReqPledgeToken: No tokens returned from CollectRBTTokens")
		return models.PledgeTokenResponse{}, fmt.Errorf("no tokens left to pledge")
	}

	pledgeResponse := models.PledgeTokenResponse{
		ReferenceId:  referenceId,
		PledgeTokens: pledgeTokenDetails,
	}

	log.Info("ReqPledgeToken: Returning pledge response", "referenceId", referenceId, "tokenCount", len(pledgeTokenDetails))
	return pledgeResponse, nil
}

func InitiateConsensus(consensusRequest models.ConsensusRequest, quorumDc types.DIDCrypto, w *wallet.Wallet, log logger.Logger) (*models.ConsensusResponse, error) {
	quorumDid := quorumDc.GetDID()
	log.Info("InitiateConsensus: Starting consensus", "quorumDID", quorumDid, "referenceId", consensusRequest.ReferenceId)

	// isTransactionInfoValidated, err := ValidateTransaction()
	// if err != nil {
	// 	log.Error("InitiateConsensus : Failed to validate transaction info", "err", err)

	// 	return &models.ConsensusResponse{}, err
	// }

	// if !isTransactionInfoValidated {
	// 	log.Error("InitiateConsensus : Transaction info validation failed")
	// 	return &models.ConsensusResponse{}, fmt.Errorf("transaction info validation failed")
	// }
	// This is the pledgeTokenInformation we need to pass to the PledgeTokens function.
	// This needs to be convered to []Tstring basically the list of token ids which are pledged for this transaction.
	pledgeDetails := consensusRequest.TransactionInfo.Quorums
	log.Debug("InitiateConsensus: Quorum details received", "quorumCount", len(pledgeDetails))

	// Here we are assuming there is only one quorum
	// This will change when multple quorums are involved
	if len(pledgeDetails) == 0 {
		log.Error("InitiateConsensus: No pledge details found in TransactionInfo")
		return &models.ConsensusResponse{}, fmt.Errorf("no pledge details found")
	}
	pledgeTokenDetails := pledgeDetails[0].Tokens
	log.Info("InitiateConsensus: Pledge tokens received from initiator", "tokenCount", len(pledgeTokenDetails))
	for i, token := range pledgeTokenDetails {
		log.Debug("InitiateConsensus: Pledge token from initiator", "index", i, "tokenID", token.TokenID, "prevTxID", token.PreviousTransactionID)
	}
	// pledgeTokenList := util.ExtractTokenIDs(pledgeTokenDetails) // Need to ensure this function logic is not existing at th emoment

	log.Debug("InitiateConsensus: Signing transaction with quorum DID", "quorumDID", quorumDid)
	quorumSignature, err := util.SignTransaction(quorumDc, consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus: Failed to sign transaction info", "quorumDID", quorumDid, "err", err)
		return &models.ConsensusResponse{}, err
	}
	log.Info("InitiateConsensus: Transaction signed successfully", "quorumDID", quorumDid, "signatureLength", len(quorumSignature))

	log.Debug("InitiateConsensus: Computing transaction ID")
	transactionId, err := util.GetTransactionID(consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus: Failed to get transaction ID", "err", err)
		return &models.ConsensusResponse{}, err
	}
	log.Info("InitiateConsensus: Transaction ID computed", "transactionID", transactionId)

	log.Debug("InitiateConsensus: Serializing transaction info")
	transactionInfoBytes, err := models.SerializeTransactionInfo(consensusRequest.TransactionInfo)
	if err != nil {
		log.Error("InitiateConsensus: Failed to serialize transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}
	log.Debug("InitiateConsensus: Transaction info serialized", "bytesLength", len(transactionInfoBytes))

	log.Debug("InitiateConsensus: Building signature structure")
	quorumSignatureInfo := models.QuorumSignature{
		Did:       quorumDid,
		Signature: quorumSignature,
	}
	log.Debug("InitiateConsensus: QuorumSignature created", "did", quorumSignatureInfo.Did, "signatureLength", len(quorumSignatureInfo.Signature))

	log.Debug("InitiateConsensus: Received initiator signature", "initiatorSignatureLength", len(consensusRequest.InitiatorSignature))
	signature := models.Signature{
		InitiatorSignature: consensusRequest.InitiatorSignature,
		Quorums:            []models.QuorumSignature{quorumSignatureInfo},
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		log.Error("InitiateConsensus: Failed to marshal signature", "err", err)
		return &models.ConsensusResponse{}, err
	}
	log.Info("InitiateConsensus: Complete signature structure created", "signatureBytesLength", len(signatureBytes))

	transactions := &models.Transactions{
		ID:        transactionId,
		Info:      transactionInfoBytes,
		Signature: signatureBytes,
	}
	log.Debug("InitiateConsensus: Transaction object created for pledging")

	consensusResponse := models.ConsensusResponse{
		ReferenceId:     consensusRequest.ReferenceId,
		QuorumSignature: quorumSignature,
		Message:         "Transaction Information verified succesfully. Consensus Complete.",
		Status:          true,
	}
	log.Info("InitiateConsensus: Consensus response prepared", "referenceId", consensusRequest.ReferenceId, "quorumSignatureLength", len(quorumSignature))

	log.Info("InitiateConsensus: Calling PledgeTokens", "tokenCount", len(pledgeTokenDetails), "transactionID", transactionId, "quorumDID", quorumDid)
	//List of Pledge token ids
	// The incoming TransactionInfo with the signature of both initiator and quorumSignature
	err = w.PledgeTokens(pledgeTokenDetails, transactions, quorumDid, int64(consensusRequest.TransactionInfo.Epoch))
	if err != nil {
		log.Error("InitiateConsensus: PledgeTokens failed", "transactionID", transactionId, "err", err)
		return &models.ConsensusResponse{}, err
	}
	log.Info("InitiateConsensus: PledgeTokens completed successfully", "transactionID", transactionId, "tokenCount", len(pledgeTokenDetails))

	log.Info("InitiateConsensus: Consensus completed successfully", "referenceId", consensusRequest.ReferenceId, "transactionID", transactionId)
	return &consensusResponse, nil
}
