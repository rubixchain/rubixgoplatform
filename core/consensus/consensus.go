package consensus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
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
	//log.Debug("ReqPledgeToken: Attempting to lock tokens", "did", dc.GetDID(), "amount", transactionValue)
	lockedTokens, err := w.LockTokensForSplit(context.Background(), dc.GetDID(), transactionValue, referenceId)
	if err != nil {
		log.Error("ReqPledgeToken: Failed to lock tokens", "err", err)
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to lock tokens for split: %w", err)
	}
	//log.Info("ReqPledgeToken: Tokens locked", "lockedTokensCount", len(lockedTokens), "transactionValue", transactionValue)
	log.Info("The transactionValue in ReqPledgeToken is ", transactionValue)

	denomMap, err := w.GetTokenDenomArray(dc.GetDID())
	if err != nil {
		log.Error("ReqPledgeToken: Failed to get denom array", "err", err)
		return models.PledgeTokenResponse{}, fmt.Errorf("ReqPledgeToken: failed to fetch token denom array: %w", err)
	}

	pledgeTokenDetails, childTokensKept, burntParentToken, mintTokensBeingBurnt, err := parts.CollectRBTTokens(
		dc,
		w,
		transactionValue,
		lockedTokens,
		denomMap,
		networkMode,
		log,
	)
	if err != nil {
		log.Error("ReqPledgeToken: CollectRBTTokens failed", "err", err)
		return models.PledgeTokenResponse{}, err
	}

	if len(childTokensKept) > 0 && len(burntParentToken) > 0 {
		genTX := childTokensKept[0].TxRecord

		// pledgeTokenDetails entries produced by a split have an empty
		// PreviousTransactionID — CollectRBTTokens builds them before it knows
		// the split's own txID. Left empty, downstream validation treats the
		// pledge/consensus transaction itself as this token's genesis instead
		// of the split transaction that actually minted it (IsParentTokenBurnt's
		// self-referential check would then look for the burnt parent in the
		// wrong transaction's CommittedTokens and fail a legitimate pledge).
		// Backfill it with genTX.ID, mirroring the same fixup
		// BuildTransactionInfoFromRequest already does for transfer tokens.
		backfilledCount := 0
		for _, t := range pledgeTokenDetails {
			if t.PreviousTransactionID == "" {
				t.PreviousTransactionID = genTX.ID
				backfilledCount++
			}
		}
		log.Debug("ReqPledgeToken: backfilled PreviousTransactionID for split-derived pledge tokens",
			"genesisTxID", genTX.ID,
			"pledgeTokenCount", len(pledgeTokenDetails),
			"backfilledCount", backfilledCount,
		)

		if errPersist := w.PersistGenesisTransaction(&wallet.PersistGenesisTransactionReq{
			DID:                  dc.GetDID(),
			GenesisTokens:        childTokensKept,
			BurnTokens:           burntParentToken,
			GenesisTransaction:   genTX,
			MintTokensBeingBurnt: mintTokensBeingBurnt,
		}); errPersist != nil {
			return models.PledgeTokenResponse{}, fmt.Errorf("reqPledgeToken: failed to persist genesis transaction, err: %v", errPersist)
		}
		if genTX.Info == nil {
			return models.PledgeTokenResponse{}, fmt.Errorf("reqPledgeToken: generated transaction info is nil")
		}

		var txInfo models.TransactionInfo
		if err := json.Unmarshal(genTX.Info, &txInfo); err != nil {
			return models.PledgeTokenResponse{}, fmt.Errorf("reqPledgeToken: failed to unmarshal transaction info, err: %v", err)
		}

		var txSingature models.Signature
		if err := json.Unmarshal(genTX.Signature, &txSingature); err != nil {
			return models.PledgeTokenResponse{}, fmt.Errorf("reqPledgeToken: failed to unmarshal signature, err: %v", err)
		}

		if networkMode != constants.NetworkMode_Localnet {
			if _, err := util.PublishTransaction(pubsub, &txInfo, &txSingature, true, ""); err != nil {
				log.Error("reqPledgeToken: failed to publish transaction, err: %v", err)
			}
		}
	}

	if len(pledgeTokenDetails) == 0 {
		log.Error("ReqPledgeToken: No tokens returned from CollectRBTTokens")
		return models.PledgeTokenResponse{}, fmt.Errorf("no tokens left to pledge")
	}

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return models.PledgeTokenResponse{}, fmt.Errorf("BuildTransactionInfoFromRequest: begin tx for adding lock reference: %w", err)
	}
	defer tx.Rollback(w.Ctx) //nolint:errcheck

	var tokenIDs []models.Token = make([]models.Token, 0)
	for _, rbtToken := range pledgeTokenDetails {
		tokenIDs = append(tokenIDs, models.Token{
			TokenID: rbtToken.TokenID,
		})
	}

	if _, err := w.AddLockReferenceForToken(w.Ctx, tx, tokenIDs, referenceId); err != nil {
		return models.PledgeTokenResponse{}, fmt.Errorf("BuildTransactionInfoFromRequest: failed to add lock reference for tokens %v: %w", tokenIDs, err)
	}

	pledgeResponse := models.PledgeTokenResponse{
		ReferenceId:  referenceId,
		PledgeTokens: pledgeTokenDetails,
	}

	pledgeTokenPrevTxIDs := make(map[string]string, len(pledgeTokenDetails))
	for _, t := range pledgeTokenDetails {
		pledgeTokenPrevTxIDs[t.TokenID] = t.PreviousTransactionID
	}
	log.Info("ReqPledgeToken: Returning pledge response",
		"referenceId", referenceId,
		"tokenCount", len(pledgeTokenDetails),
		"pledgeTokenPrevTxIDs", pledgeTokenPrevTxIDs,
	)
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
