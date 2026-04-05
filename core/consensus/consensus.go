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

// InitiateConsensus validates and signs the consensus request on behalf of the
// quorum node. It returns the ConsensusResponse, the computed transactionId,
// and any error.
//
// This function is pure: it does NOT call PledgeV2. The caller
// (initiateConsensusHandler in core/quorum_initiator.go) is responsible for
// calling c.PledgeV2 after this function returns successfully.
func InitiateConsensus(consensusRequest models.ConsensusRequest, quorumDc types.DIDCrypto, log logger.Logger) (*models.ConsensusResponse, error) {
	txnInfo := consensusRequest.TransactionInfo

	// Validate pledge details exist.
	pledgeDetails := txnInfo.Quorums
	if len(pledgeDetails) == 0 {
		log.Error("InitiateConsensus : No pledge details found")
		return &models.ConsensusResponse{}, fmt.Errorf("no pledge details found")
	}

	quorumSignature, err := util.SignTransaction(quorumDc, txnInfo)
	if err != nil {
		log.Error("InitiateConsensus : Failed to sign transaction info", "err", err)
		return &models.ConsensusResponse{}, err
	}

	consensusResponse := models.ConsensusResponse{
		ReferenceId:     consensusRequest.ReferenceId,
		QuorumSignature: quorumSignature,
		Message:         "Transaction Information verified succesfully. Consensus Complete.",
		Status:          true,
	}

	return &consensusResponse, nil
}
