package consensus

import (
	"context"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
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
func initiateConsensus() {
	//This must be called at the end of initiateConsensus

	// c.w.AddTokenStateHashes( < current token state hash>)
}
