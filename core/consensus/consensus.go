package consensus

import (
	"context"
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

// InitiateConsensus validates and signs the consensus request on behalf of the
// quorum node. It returns the ConsensusResponse and any error.
//
// This function is pure: it does NOT call PledgeV2. The caller
// (initiateConsensusHandler in core/quorum_initiator.go) is responsible for
// calling c.PledgeV2 after this function returns successfully.
func InitiateConsensus(consensusRequest models.ConsensusRequest, quorumDc types.DIDCrypto, log logger.Logger) (*models.ConsensusResponse, error) {
	txnInfo := consensusRequest.TransactionInfo

	log.Info("InitiateConsensus: Starting consensus", "quorumDID", quorumDc.GetDID(), "referenceId", consensusRequest.ReferenceId)

	// Validate pledge details exist
	pledgeDetails := txnInfo.Quorums
	if len(pledgeDetails) == 0 {
		log.Error("InitiateConsensus: No pledge details found in TransactionInfo")
		return &models.ConsensusResponse{}, fmt.Errorf("no pledge details found")
	}

	log.Debug("InitiateConsensus: Signing transaction with quorum DID", "quorumDID", quorumDc.GetDID())
	quorumSignature, err := util.SignTransaction(quorumDc, txnInfo)
	if err != nil {
		log.Error("InitiateConsensus: Failed to sign transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}

	consensusResponse := models.ConsensusResponse{
		ReferenceId:     consensusRequest.ReferenceId,
		QuorumSignature: quorumSignature,
		Message:         "Transaction Information verified succesfully. Consensus Complete.",
		Status:          true,
	}

	log.Info("InitiateConsensus: Consensus completed successfully", "referenceId", consensusRequest.ReferenceId)
	return &consensusResponse, nil
}
