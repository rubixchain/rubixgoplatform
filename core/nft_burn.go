package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// validateNFTBurnRequest checks that an NFT burn request is well-formed and
// that every NFT named in it may legally be burnt by the caller.
//
// A burn is a terminal, non-consensus operation, so the guards here are the
// only thing standing between a user and permanent destruction of an asset —
// there is no quorum validation downstream to catch a bad request.
//
// Returns the set of NFT IDs that are already burnt (so the caller can treat a
// fully-redundant request as an idempotent success rather than an error).
func (c *Core) validateNFTBurnRequest(request *models.TransactionRequest) (alreadyBurnt map[string]struct{}, err error) {
	alreadyBurnt = make(map[string]struct{})

	// A burn is standalone: mixing it with value transfer would mean the burn
	// takes the non-consensus path while the RBT/FT side silently loses its
	// quorum validation.
	if request.Tokens.RBT > 0 || len(request.Tokens.FT) > 0 || len(request.Tokens.SmartContract) > 0 {
		return nil, fmt.Errorf("burnNft cannot be combined with RBT, FT or smart contract transfers")
	}
	if request.Tokens.TransferNFTOwnership {
		return nil, fmt.Errorf("burnNft cannot be combined with transferNftOwnership")
	}
	// Burn is exempt from properties enforcement: its guards are local checks
	// on the owner's own node, so any gate here is bypassable by a modified
	// client. Blocking a burn would also re-strand the quorum collateral this
	// feature exists to release. This guard only stops a properties token being
	// smuggled into a burn request.
	if request.Tokens.SetProperties || request.Tokens.Properties != nil {
		return nil, fmt.Errorf("burnNft cannot be combined with setProperties")
	}

	ownerDID := request.Initiator
	if ownerDID == "" {
		return nil, fmt.Errorf("burnNft: initiator DID is required")
	}

	nftTypeID := int16(models.GetTokenTypeID(constants.TokenType_NFT))

	for _, nftInfo := range request.GetAllNFTs() {
		nftID := nftInfo.NFTId
		if nftID == "" {
			return nil, fmt.Errorf("burnNft: nftId is required for every NFT entry")
		}
		if nftInfo.ParentNFTId != "" {
			return nil, fmt.Errorf("burnNft: cannot burn and child-mint in the same request (nft %s)", nftID)
		}

		token, tokenErr := c.w.GetTokenByTokenID(nftID)
		if tokenErr != nil {
			return nil, fmt.Errorf("burnNft: NFT %s not found: %w", nftID, tokenErr)
		}

		if token.TokenType != nftTypeID {
			return nil, fmt.Errorf("burnNft: token %s is not an NFT", nftID)
		}

		if token.DID != ownerDID {
			return nil, fmt.Errorf("burnNft: NFT %s is owned by %s, not by initiator %s",
				nftID, token.DID, ownerDID)
		}

		// Already burnt — record it and skip the remaining guards so a repeated
		// request is idempotent rather than an error.
		if token.TokenStatus == int16(constants.TokenStatus_Burnt) {
			alreadyBurnt[nftID] = struct{}{}
			continue
		}

		// A parent NFT must not be burnt while it still has live children:
		// GetParentNFT would resolve to a destroyed token and the children
		// would be orphaned.
		liveChildren, childErr := c.liveChildNFTs(nftID)
		if childErr != nil {
			return nil, fmt.Errorf("burnNft: failed to check children of NFT %s: %w", nftID, childErr)
		}
		if len(liveChildren) > 0 {
			return nil, fmt.Errorf(
				"burnNft: NFT %s is a parent with %d live child NFT(s) [%s] — burn the children first",
				nftID, len(liveChildren), strings.Join(liveChildren, ", "))
		}

		// Free/Deployed/Executed are the states an idle NFT rests in; anything
		// else means it is not safely destroyable right now. In practice the
		// state this excludes is Locked (a transaction is mid-flight) — NFTs are
		// never Pledged, since pledging selects RBT only via CollectRBTTokens.
		// The switch stays default-deny rather than listing exclusions, so any
		// future status is refused until someone decides it is burnable.
		switch token.TokenStatus {
		case int16(constants.TokenStatus_Free),
			int16(constants.TokenStatus_Deployed),
			int16(constants.TokenStatus_Executed):
			// burnable
		default:
			return nil, fmt.Errorf("burnNft: NFT %s has status %d and cannot be burnt (want Free, Deployed or Executed)",
				nftID, token.TokenStatus)
		}
	}

	return alreadyBurnt, nil
}

// finalizeNFTBurn completes an NFT burn without going through consensus.
//
// It mirrors the tail of initiateTransaction — compute the transaction ID, sign
// it, persist, publish — but skips the pledge request, quorum selection and
// consensus round entirely. A burn destroys value rather than moving it, so
// there is nothing for a quorum to conserve; requiring a pledge would also
// deadlock the feature, since an NFT burn has a transaction value of 0 and
// would be rejected as "insufficient quorum liquidity".
//
// The resulting transaction carries no quorum signature. Receiving nodes accept
// it on the initiator's signature alone.
//
// Publishing order matters and mirrors initiateTransaction:
//  1. the NFT's own topic, so subscribers stop treating the NFT as live
//  2. the rubix_txn stream, so quorums that pledged against this NFT match its
//     PreviousTransactionID in unpledge_sequence_info and release collateral
//
// Both publishes happen only after the DB commit succeeds — broadcasting a burn
// that then failed to persist would tell the network an NFT is dead while this
// node still believes it is live.
//
// KNOWN LIMITATION — offline subscribers: pubsub delivery is fire-and-forget,
// so a subscriber that is down when the burn is published never receives the
// event and keeps reporting the NFT as live. This is not specific to burns —
// every NFT execute/transfer event has the same property — and it is not
// silently unsafe: a stale subscriber that tries to spend the burnt NFT is
// rejected at consensus by TokenChainIntegrityCheck, because its chain tip no
// longer matches. A node that subscribes later picks the burn up via
// SubscribeNFTSetup's chain sync. Accepted deliberately rather than solved with
// a startup reconciliation pass.
func (c *Core) finalizeNFTBurn(
	ctx context.Context,
	reqID string,
	dc types.DIDCrypto,
	transactionInfo *models.TransactionInfo,
	initiatorDID string,
	txSucceeded *bool,
) *models.BasicResponse {
	resp := &models.BasicResponse{Status: false}

	burnTxID, err := util.GetTransactionID(transactionInfo)
	if err != nil {
		c.log.Error("finalizeNFTBurn: failed to compute transaction ID", "err", err)
		resp.Message = "finalizeNFTBurn: failed to compute transaction ID: " + err.Error()
		return resp
	}

	initiatorSignature, err := util.SignTransaction(dc, transactionInfo)
	if err != nil {
		c.log.Error("finalizeNFTBurn: failed to sign burn transaction", "err", err, "did", initiatorDID)
		resp.Message = "finalizeNFTBurn: failed to sign burn transaction: " + err.Error()
		return resp
	}

	signature := &models.Signature{InitiatorSignature: initiatorSignature}

	txInfoBytes, err := models.SerializeTransactionInfo(transactionInfo)
	if err != nil {
		c.log.Error("finalizeNFTBurn: failed to serialize transaction info", "err", err)
		resp.Message = "finalizeNFTBurn: failed to serialize transaction info: " + err.Error()
		return resp
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		c.log.Error("finalizeNFTBurn: failed to marshal signature", "err", err)
		resp.Message = "finalizeNFTBurn: failed to marshal signature: " + err.Error()
		return resp
	}

	burnTx := &models.Transactions{
		ID:        burnTxID,
		Info:      txInfoBytes,
		Signature: signatureBytes,
	}

	nftIDs := make([]string, 0, len(transactionInfo.Tokens.NFT))
	for _, nftToken := range transactionInfo.Tokens.NFT {
		nftIDs = append(nftIDs, nftToken.TokenID)
	}

	if err := c.w.PersistNFTBurn(ctx, &wallet.PersistNFTBurnRequest{
		DID:             initiatorDID,
		NFTIDs:          nftIDs,
		BurnTransaction: burnTx,
	}); err != nil {
		c.log.Error("finalizeNFTBurn: failed to persist NFT burn", "err", err, "burnTxID", burnTxID)
		resp.Message = "finalizeNFTBurn: failed to persist NFT burn: " + err.Error()
		return resp
	}

	// The burn is committed. Disarm the caller's deferred cleanup so it does
	// not release tokens that no longer need releasing, and free the reference.
	*txSucceeded = true
	if err := c.w.ReleaseReferenceID(reqID); err != nil {
		c.log.Error("finalizeNFTBurn: failed to release reference ID", "err", err, "reqID", reqID)
	}

	c.log.Info("finalizeNFTBurn: NFT burn persisted",
		"burnTxID", burnTxID, "did", initiatorDID, "nftIDs", nftIDs)

	// Notify subscribers on each NFT's own topic.
	for _, nftID := range nftIDs {
		burnEvent := &models.EventNFTPublishInfo{
			NFTid:              nftID,
			TransactionID:      burnTxID,
			Initiator:          initiatorDID,
			InitiatorSignature: initiatorSignature,
			Epoch:              transactionInfo.Epoch,
			IsBurn:             true,
		}
		if err := c.publishNewNftEvent(burnEvent); err != nil {
			c.log.Error("finalizeNFTBurn: failed to publish NFT burn event",
				"nft_token", nftID, "err", err)
		}
	}

	// Notify the network so pledging quorums can release their collateral.
	if _, err := util.PublishTransaction(c.ps, transactionInfo, signature, true, ""); err != nil {
		c.log.Error("finalizeNFTBurn: failed to publish burn transaction", "err", err, "burnTxID", burnTxID)
	}

	resp.Status = true
	resp.Message = fmt.Sprintf("NFT burn successful, transaction id: %s", burnTxID)
	return resp
}

// liveChildNFTs returns the IDs of child NFTs of parentNFTId that have not
// themselves been burnt. Children that are already burnt do not block their
// parent, otherwise a fully-burnt subtree would pin its root forever.
func (c *Core) liveChildNFTs(parentNFTId string) ([]string, error) {
	children, err := c.w.GetChildNFTs(parentNFTId)
	if err != nil {
		return nil, err
	}

	live := make([]string, 0, len(children))
	for _, child := range children {
		if child.TokenStatus == int16(constants.TokenStatus_Burnt) {
			continue
		}
		live = append(live, child.TokenID)
	}

	return live, nil
}
