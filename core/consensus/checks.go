package consensus

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	RBTString  string = "rbt"
	PartString string = "part"
)

var validNetworks = map[string]struct{}{
	constants.NetworkMode_Mainnet:  {},
	constants.NetworkMode_Testnet:  {},
	constants.NetworkMode_Localnet: {},
}

var alphanumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]*$`)

func validateDID(did string, label string) error {
	if !alphanumericRegex.MatchString(did) {
		return fmt.Errorf("invalid %s DID %q: must contain only alphanumeric characters", label, did)
	}
	if !strings.HasPrefix(did, constants.DidPrefix) {
		return fmt.Errorf("invalid %s DID %q: must start with %q", label, did, constants.DidPrefix)
	}
	if len(did) != constants.DidLength {
		return fmt.Errorf("invalid %s DID %q: length must be %d, got %d", label, did, constants.DidLength, len(did))
	}
	return nil
}

func ValidateTransactionInfoFields(txnInfo *models.TransactionInfo) error {
	if err := validateDID(txnInfo.Initiator, "initiator"); err != nil {
		return err
	}

	// Owner DID is optional for deployment transactions (SC/NFT deployment)
	// It should only be validated if it's not empty
	if txnInfo.Owner != "" {
		if err := validateDID(txnInfo.Owner, "owner"); err != nil {
			return err
		}
	}

	currentEpoch := int(time.Now().Unix())
	if txnInfo.Epoch <= 0 || txnInfo.Epoch < constants.MinTransactionEpochUnix || txnInfo.Epoch > currentEpoch {
		return fmt.Errorf("invalid epoch %d: must be a positive Unix time on or after 2026-04-01 00:00:00 UTC (epoch %d) and not after the current time (epoch %d)",
			txnInfo.Epoch, constants.MinTransactionEpochUnix, currentEpoch)
	}

	if _, ok := validNetworks[txnInfo.Network]; !ok {
		return fmt.Errorf("invalid network %q: must be one of the supported networks", txnInfo.Network)
	}

	if txnInfo.Tokens == nil ||
		(len(txnInfo.Tokens.RBT) == 0 && len(txnInfo.Tokens.NFT) == 0 &&
			len(txnInfo.Tokens.FT) == 0 && len(txnInfo.Tokens.SmartContract) == 0) {
		return fmt.Errorf("transaction must contain at least one transfer token (RBT, NFT, FT, or SmartContract)")
	}

	return nil
}

func TransactionIDIntegrityCheck(transactionID string, transactionInfo *models.TransactionInfo) error {
	computedTransactionID, err := util.GetTransactionID(transactionInfo)
	if err != nil {
		return fmt.Errorf("failed to compute transaction ID: %w", err)
	}
	if computedTransactionID != transactionID {
		return fmt.Errorf("transaction ID mismatch: computed %s != provided %s", computedTransactionID, transactionID)
	}
	return nil
}

// SignatureVerificationCheck validates the initiator and quorum signatures.
// initiatorDC is the DIDCrypto for the initiator, and quorumDCs maps each
// quorum DID to its DIDCrypto (pre-initialised by the caller).
func SignatureVerificationCheck(
	txInfo *models.TransactionInfo,
	sig *models.Signature,
	initiatorDC types.DIDCrypto,
	quorumDCs map[string]types.DIDCrypto,
) error {
	if err := util.VerifySignature(initiatorDC, txInfo, sig.InitiatorSignature); err != nil {
		return fmt.Errorf("SignatureVerificationCheck: initiator signature verification failed: %w", err)
	}
	for _, quorumSig := range sig.Quorums {
		quorumDC, ok := quorumDCs[quorumSig.Did]
		if !ok {
			return fmt.Errorf("SignatureVerificationCheck: no DIDCrypto provided for quorum %s", quorumSig.Did)
		}
		if err := util.VerifySignature(quorumDC, txInfo, quorumSig.Signature); err != nil {
			return fmt.Errorf("SignatureVerificationCheck: quorum %s signature verification failed: %w", quorumSig.Did, err)
		}
	}
	return nil
}

func ValidateTokenOwnershipByPrevTxn(txnInfo *models.TransactionInfo, isFullnode bool, w *wallet.Wallet) error {
	if txnInfo.Tokens == nil {
		return nil
	}

	prevTxnTokensMap := make(map[string][]string)

	// Only RBT and FT tokens are checked for ownership by previous transaction.
	// NFT and SmartContract tokens are excluded because the Owner field in
	// TransactionInfo represents the RBT counterparty/receiver, not the NFT/SC
	// token owner. For NFT/SC execution (non-ownership-transfer), any DID with
	// access can execute — the initiator does not need to match the previous
	// transaction's Owner. Ownership enforcement for NFT transfers is handled
	// at the initiator node via QueryAndLockForExecution's checkOwnership flag.
	// So the onwership validation for NFTs will only be a matter when the
	// Transfer ownership flag is true. Which we are not passing in here. Which needs to be passed inorder to validate the ownership transfer.
	tokenLists := [][]*models.TokenInfo{
		txnInfo.Tokens.RBT,
		txnInfo.Tokens.FT,
	}

	for _, tokens := range tokenLists {
		for _, t := range tokens {
			if t.PreviousTransactionID == "" {
				continue //skip the check for minting transactions
			}
			prevTxnTokensMap[t.PreviousTransactionID] = append(
				prevTxnTokensMap[t.PreviousTransactionID], t.TokenID,
			)
		}
	}

	for prevTxnID, tokenIDs := range prevTxnTokensMap {
		//lets call prevTx as tx2
		prevTx, err := w.GetTransactionByID(prevTxnID, isFullnode)
		if err != nil {
			return fmt.Errorf("failed to fetch previous transaction %s: %w", prevTxnID, err)
		}
		if prevTx == nil {
			return fmt.Errorf("previous transaction %s not found for tokens %v", prevTxnID, tokenIDs)
		}
		var prevTxnInfo models.TransactionInfo
		if err := json.Unmarshal(prevTx.Info, &prevTxnInfo); err != nil {
			return fmt.Errorf("failed to unmarshal info of previous transaction %s: %w", prevTxnID, err)
		}

		//for each tokenID,get the role of the token which is there in previous transaction from the token chain table.
		//If role is unpledged, then we need check 2 things
		//a. get the role of the token in the txn which prev to tx2.
		//b. we need to check that tx1 &tx2 both are having atleast 1 token in common, tx2 should be the transacion of atleast 1 token of tx1.

		for _, tokenID := range tokenIDs {
			//lets call the previous transaction as tx2.
			tokenChainForToken, err := w.GetTokenChainByTokenID(tokenID, isFullnode)
			if err != nil {
				return fmt.Errorf("ValidateTokenOwnershipByPrevTxn: failed to get latest role for token %s: %w", tokenID, err)
			}
			if len(tokenChainForToken) == 0 {
				return fmt.Errorf("ValidateTokenOwnershipByPrevTxn: token chain not found for token %s, possible failure in syncing of the token", tokenID)
			}

			latestTokenChain := tokenChainForToken[len(tokenChainForToken)-1]

			// If role is unpledged, we validate the whether the unpledge done is valid
			if latestTokenChain.Role == int16(models.GetTokenRoleID(constants.TokenRole_Unpledge)) {
				// There should be atleast a pledge and an unpledge block in the token chain for that token,
				// because if the role is unpledged, then it means that token should have been pledged before
				// and then unpledged in the previous transaction.
				if len(tokenChainForToken) < 2 {
					return fmt.Errorf("ValidateTokenOwnershipByPrevTxn: insufficient token chain entries for token %s to verify unpledged token's ownership", tokenID)
				}

				previousTransactionID := latestTokenChain.PreviousTransactionID
				if previousTransactionID == nil {
					return fmt.Errorf("ValidateTokenOwnershipByPrevTxn: previous transaction ID is nil for token %s that is unpledged", tokenID)
				}

				pledgedTxn, err := w.GetTransactionByID(*previousTransactionID, isFullnode)
				if err != nil {
					return fmt.Errorf("failed to get pledged transaction %s: %w", *previousTransactionID, err)
				}
				if pledgedTxn == nil {
					return fmt.Errorf("pledged transaction %s not found", *previousTransactionID)
				}
				var pledgedTxnInfo models.TransactionInfo
				if err := json.Unmarshal(pledgedTxn.Info, &pledgedTxnInfo); err != nil {
					return fmt.Errorf("failed to unmarshal info of pledged transaction %s: %w", *previousTransactionID, err)
				}

				// The unpledged token must exist in the original pledge transaction's quorum tokens.
				if !isTokenPledged(&pledgedTxnInfo, tokenID) {
					return fmt.Errorf("token %s is marked unpledged but not found in quorum tokens of pledged transaction %s", tokenID, *previousTransactionID)
				}
				// tx1 and tx2 must have at least one common token.
				if !getTransactionTokens(&pledgedTxnInfo, &prevTxnInfo) {
					return fmt.Errorf("no common transaction tokens found between pledged transaction %s and previous transaction %s", *previousTransactionID, prevTxnID)
				}
			} else {
				//If previous txn details are not there in the fullnode side, sync those transaction details from the initiator.
				//In general, Previous transaction won't be nil, because we are calling this function after the sync in the  TokenChainIntigretyCheck function.
				if prevTxnInfo.Owner != txnInfo.Initiator {
					return fmt.Errorf(
						"ownership mismatch: initiator %s does not match owner %s of previous transaction %s (affected tokens: %v)",
						txnInfo.Initiator, prevTxnInfo.Owner, prevTxnID, tokenIDs,
					)
				}

			}

		}
	}

	//if isFullnode, Fullnode checks that owner of the previous transaction is same as the quorum which has pledged this token
	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				tokenDetails, err := w.GetFullNodeRBTToken(t.TokenID)
				if err != nil {
					return fmt.Errorf("failed to get fullnode RBT token %s: %w", t.TokenID, err)
				} // Handle the case where there is no RBT token in the fullnode rbt tokens table,
				//If we call, ValidateTokenOwnershipByPrevTxn function after calling the TokenChainIntigrityCheck function,
				//There is no need to sync the transaction chain from the peer, because the TokenChainIntigrityCheck function will sync the transaction chain from the peer.
				previousTransactionOwner := tokenDetails.DID
				if previousTransactionOwner != quorum.Did {
					return fmt.Errorf("ValidateTokenOwnershipByPrevTxn: ownership mismatch, quorum %s does not match owner %s of previous transaction %s", quorum.Did, tokenDetails.DID, t.TokenID)
				}
			}
		}
	}
	return nil

}
func ValidateNewTokenContent(tokenID string, isQuorum bool, testnet bool, mainnet bool, localnet bool, log logger.Logger) error {
	devidedParts := strings.Split(tokenID, "_")

	tokenTypeString := RBTString
	if len(devidedParts) == 3 {
		tokenTypeString = PartString
	}

	// level is the token-mapping level (e.g. 10000 + mapLevel for localnet tokens).
	// This is NOT the denom-tree level (0-6). Here we subtract the network offset to get the TokenMap lookup key.
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}

	tokenNo, err := strconv.Atoi(devidedParts[1])
	if err != nil {
		return fmt.Errorf("invalid token number in token content: %s", tokenID)
	}

	shouldValidate := testnet || mainnet || localnet

	if shouldValidate {
		mapLevel := level
		network := constants.NetworkMode_Mainnet

		if testnet {
			network = constants.NetworkMode_Testnet
			if level < constants.TestnetRBT_Level_Offset {
				return fmt.Errorf(
					"invalid testnet token level %d: testnet level must be >= %d",
					level, constants.TestnetRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.TestnetRBT_Level_Offset
		} else if localnet {
			network = constants.NetworkMode_Localnet
			if level < constants.LocalRBT_Level_Offset {
				return fmt.Errorf(
					"invalid local token level %d: localnet level must be > %d",
					level, constants.LocalRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.LocalRBT_Level_Offset
		}

		maxAllowed, ok := token.TokenMap[mapLevel]
		if !ok {
			return fmt.Errorf(
				"invalid %s token level %d: map level %d not present in TokenMap",
				network, level, mapLevel,
			)
		}
		if tokenNo < 0 || tokenNo > maxAllowed {
			return fmt.Errorf(
				"%s token number %d exceeds max allowed %d for level %d",
				network, tokenNo, maxAllowed, level,
			)
		}
		//	log.Debug("token content validated for "+network, tokenID)
	}

	MaxPossiblePartTokenNumber := parts.MaxPossiblePartsIndexByMaxDecimalPlaces(uint(constants.MaxSupportedDecimalPlaces))
	if tokenTypeString == PartString {
		partTokenNumber, err := strconv.Atoi(devidedParts[2])
		if err != nil {
			return fmt.Errorf("invalid part number in token content: %s", tokenID)
		}
		if partTokenNumber > MaxPossiblePartTokenNumber {
			return fmt.Errorf(
				"Parttoken number %d exceeds max allowed %d ",
				partTokenNumber, MaxPossiblePartTokenNumber,
			)
		}
		//log.Debug("token content validated for the part token", tokenID)
	}

	return nil
}

// 1. First check whether quorum is involved in the transaction or not, if quorum is not involved don't do this check.
// If quorum is involved, then check whether RBTs are getting transferred or not, if RBTs are getting transferred, then check the transaction value should be less than or equal to the pledge value.
// burnt tokens also will be there in committed tokens list, so transaction value and committed tokens value wont' match.
// 2. whatever the token value which initiator is passing, only in case if RBTs fullnode can check their actual value
// by looking at the tokenID, for other assets fullnode can't get the actual value.
// So In this function fullnode will be validating only for RBTs.
func ValidateTransactionValueAndPledge(txnInfo *models.TransactionInfo) error {
	if txnInfo.Tokens == nil {
		return fmt.Errorf("transaction has no tokens")
	}

	// First check whether quorum is involved in the transaction or not, if quorum is not involved don't do this check.
	if len(txnInfo.Quorums) == 0 {
		return nil
	}

	transactionValue := rubixmath.ZeroFloat()

	// Count RBT tokens (verifiable from token ID)
	for _, t := range txnInfo.Tokens.RBT {
		tokenValue, err := util.GetTokenValueFromTokenID(t.TokenID)
		if err != nil {
			return fmt.Errorf("failed to get value for RBT token %s: %w", t.TokenID, err)
		}
		transactionValue = rubixmath.AddFloat(transactionValue, tokenValue)
	}

	// NOTE: NFT and SmartContract value checks are commented out for now.
	// Fullnode can only independently verify RBT values from token IDs.
	// NFT/SC values are self-reported by the initiator and cannot be cross-checked.
	// Uncomment when independent value verification is available for these asset types.

	// // Count NFT token values
	// // NFT value is optional - if not set or 0, default pledge requirement is 1 RBT
	// for _, t := range txnInfo.Tokens.NFT {
	// 	var tokenValue float64
	// 	if t.TokenValue > 0 {
	// 		tokenValue = t.TokenValue
	// 	} else {
	// 		// NFT with no explicit value requires 1 RBT pledge
	// 		tokenValue = 1.0
	// 	}
	// 	transactionValue = rubixmath.AddFloat(transactionValue, tokenValue)
	// }

	// // Count SmartContract token values
	// // SC value is mandatory and must be present in TokenValue field
	// for _, t := range txnInfo.Tokens.SmartContract {
	// 	if t.TokenValue <= 0 {
	// 		return fmt.Errorf("SmartContract token %s has invalid value %v: value must be greater than 0", t.TokenID, t.TokenValue)
	// 	}
	// 	transactionValue = rubixmath.AddFloat(transactionValue, t.TokenValue)
	// }

	totalPledgeValue := rubixmath.ZeroFloat()
	for _, quorum := range txnInfo.Quorums {
		for _, t := range quorum.Tokens {
			tokenValue, err := util.GetTokenValueFromTokenID(t.TokenID)
			if err != nil {
				return fmt.Errorf("failed to get value for pledge token %s from quorum %s: %w", t.TokenID, quorum.Did, err)
			}
			totalPledgeValue = rubixmath.AddFloat(totalPledgeValue, tokenValue)
		}
	}

	if transactionValue > totalPledgeValue {
		return fmt.Errorf(
			"transaction value (%v) is more than the total pledge amount (%v)",
			transactionValue, totalPledgeValue,
		)
	}

	return nil
}

func IsParentTokenBurnt(
	isFullNode bool,
	tokenID string,
	currentTokenOwner string,
	w *wallet.Wallet,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
) (error, bool) {
	var parentTokenID string
	var err error

	partTokenID := util.TokenID(tokenID)
	parentTokenID, err = partTokenID.GetParentToken()
	if err != nil {
		return fmt.Errorf("IsParentTokenBurnt: failed to get parent id of token %s: %w", partTokenID, err), false
	}
	if parentTokenID == "" {
		return fmt.Errorf("IsParentTokenBurnt: failed to compute parent id of token %s: %w", partTokenID, err), false
	}

	var genesisTx *models.Transactions
	if isFullNode {
		genesisTx, _, err = w.GetFullNodeTransactionAndRoleAtHeight(tokenID, 0)
	} else {
		genesisTx, _, err = w.GetTransactionAndRoleAtHeight(tokenID, 0)
	}
	if err != nil {
		// sync token chain from initiator
		err = syncTxChains(currentTokenOwner, []string{tokenID}, map[string]string{tokenID: ""}, []string{})
		if err != nil {
			return fmt.Errorf("failed to get genesis transaction for token %s: %w", tokenID, err), false
		} else {
			if isFullNode {
				genesisTx, _, err = w.GetFullNodeTransactionAndRoleAtHeight(tokenID, 0)
			} else {
				genesisTx, _, err = w.GetTransactionAndRoleAtHeight(tokenID, 0)
			}
		}
	}
	if genesisTx == nil {
		return fmt.Errorf("genesis transaction not found for token %s", tokenID), false
	}

	var txInfo models.TransactionInfo
	if err := json.Unmarshal(genesisTx.Info, &txInfo); err != nil {
		return fmt.Errorf("failed to unmarshal transaction info for token %s: %w", tokenID, err), false
	}

	for _, committedToken := range txInfo.CommittedTokens {
		if committedToken.TokenID == parentTokenID {
			return nil, true
		}
	}

	return nil, false
}

func ValidateGenuineTokenCreator(tokenID string, isFullNode bool, w *wallet.Wallet) error {
	devidedParts := strings.Split(tokenID, "_")

	// level is the token-mapping level (e.g. 10001 for localnet), NOT the denom-tree level (0-6).
	// NOTE: The check below (level == 1) appears to be a legacy guard for an older token format
	// that predates the 10000-offset scheme. It is dead code for tokens with level >= 10001.
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}
	if level == 1 {
		genesisTx, err := w.GetGenesisTransactionIdByTokenId(tokenID, isFullNode)
		if err != nil {
			return fmt.Errorf("failed to get genesis transaction for token %s: %w", tokenID, err)
		}
		genesisTxInfo, err := w.GetTransactionByID(genesisTx, isFullNode)
		if err != nil {
			return fmt.Errorf("failed to get genesis transaction info for token %s: %w", tokenID, err)
		}
		if genesisTxInfo == nil {
			return fmt.Errorf("genesis transaction not found for token %s", tokenID)
		}
		//TODO: Get the owner from the genesis transaction info and check whether he is eligible to create the token.
	}

	return nil
}

func ValidateTokenIDRelatedChecks(
	tokenID string,
	currentTokenOwner string,
	isFullNode bool,
	w *wallet.Wallet,
	testnet bool,
	mainnet bool,
	localnet bool,
	log logger.Logger,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
) error {
	err := ValidateNewTokenContent(tokenID, isFullNode, testnet, mainnet, localnet, log)
	if err != nil {
		return fmt.Errorf("failed to validate token content: %w", err)
	}
	//call IsParentTokenBurnt
	//First check whether the token is a part token or not. If it is a part token, then check whether the parent token is burnt.
	devidedParts := strings.Split(tokenID, "_")
	if len(devidedParts) == 3 {
		err, isParentTokenBurnt := IsParentTokenBurnt(isFullNode, tokenID, currentTokenOwner, w, syncTxChains)
		if err != nil {
			return fmt.Errorf("failed to validate parent token burnt: %w", err)
		}
		if !isParentTokenBurnt {
			return fmt.Errorf("parent token is not burnt")
		}
	}
	// err = ValidateGenuineTokenCreator(tokenID, isFullNode, w)
	// if err != nil {
	// 	return fmt.Errorf("failed to validate genuine token creator: %w", err)
	// }
	return nil
}

// func SyncTransactionChainFrom(p *ipfsport.Peer, tokenID string, w *wallet.Wallet, log logger.Logger) (error, *models.TransactionChainSyncReply) {
// 	var err error

// 	latestTransactionID := w.GetLatestTransactionID(tokenID)
// 	if latestTransactionID == "" {
// 		log.Error("failed to get latest transaction id")
// 		return err, nil
// 	}

// 	syncReq := models.TransactionChainSyncRequest{
// 		TokenID:       tokenID,
// 		TransactionID: latestTransactionID,
// 	}

// 	for {
// 		var trep models.TransactionChainSyncReply
// 		err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &syncReq, &trep, false)
// 		if err != nil {
// 			log.Error("failed to sync transaction chain")
// 			return err, nil
// 		}
// 		if !trep.Status {
// 			log.Error("failed to sync transaction chain")
// 			return fmt.Errorf(trep.Message), nil
// 		}
// 		if len(trep.Transactions) > 0 {
// 			for _, txn := range trep.Transactions {
// 				tx, err := util.TransactionFromBytes(txn)
// 				if tx == nil {
// 					log.Error("failed to convert transaction bytes to transaction")
// 					return fmt.Errorf("failed to convert transaction bytes to transaction"), nil
// 				}
// 				var txInfo models.TransactionInfo
// 				if err = json.Unmarshal(tx.Info, &txInfo); err != nil {
// 					log.Error("failed to unmarshal transaction info", "err", err)
// 					return fmt.Errorf("failed to unmarshal transaction info: %w", err), nil
// 				}

// 				role := FindTokenRoleInTxn(tokenID, &txInfo)

// 				if err = w.CreateTransaction(tx); err != nil {
// 					log.Error("failed to add transaction to transactions table", "err", err)
// 					return fmt.Errorf("failed to add transaction: %w", err), nil
// 				}

// 				tokenDetails, err := w.GetTokenByTokenID(tokenID)
// 				if err != nil {
// 					newToken := models.Token{
// 						TokenID:        tokenID,
// 						TokenStatus:    constants.TokenStatus_Free,
// 						DID:            txInfo.Owner,
// 						TransactionID:  tx.ID,
// 						TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
// 						LatestPosition: 0,
// 						LatestRole:     role,
// 						CreatedAt:      time.Now(),
// 						UpdatedAt:      time.Now(),
// 					}
// 					if createErr := w.CreateRBTToken(newToken); createErr != nil {
// 						log.Error("failed to create token", "err", createErr)
// 						return fmt.Errorf("failed to create token: %w", createErr), nil
// 					}
// 					tokenDetails = newToken
// 				} else {
// 					tokenDetails.DID = txInfo.Owner
// 					tokenDetails.TransactionID = tx.ID
// 					tokenDetails.LatestPosition++
// 					tokenDetails.LatestRole = role
// 					if updateErr := w.UpdateToken(tokenDetails); updateErr != nil {
// 						log.Error("failed to update token", "err", updateErr)
// 						return fmt.Errorf("failed to update token: %w", updateErr), nil
// 					}
// 				}

// 				entry := &models.TokenChain{
// 					TokenID:       tokenID,
// 					TransactionID: tx.ID,
// 					Role:          role,
// 					Position:      tokenDetails.LatestPosition,
// 				}
// 				if err = w.AddTokenChainEntry(entry); err != nil {
// 					log.Error("failed to add token chain entry", "err", err)
// 					return fmt.Errorf("failed to add token chain entry: %w", err), nil
// 				}
// 			}
// 		}
// 		if trep.NextTransactionID == "" {
// 			break
// 		}
// 		syncReq.TransactionID = trep.NextTransactionID
// 	}
// 	return nil, nil
// }

// TokenChainIntegrityCheck verifies that every token's local chain tip matches
// the incoming transaction's PreviousTransactionID. When a mismatch or missing
// token is found, it triggers a sync via syncTxChains (injected from core to
// avoid cyclic imports) and then re-verifies. If even a single token still
// doesn't match after sync, the check fails.
func TokenChainIntegrityCheck(
	txnInfo *models.TransactionInfo,
	currentTxID string,
	isFullnode bool,
	w *wallet.Wallet,
	log logger.Logger,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
) error {
	if txnInfo.Tokens == nil {
		return nil
	}

	// Collect all tokens that need checking: transfer tokens + pledged tokens (fullnode only).
	type tokenCheck struct {
		tokenID               string
		previousTransactionID string
		syncFromPeerDID       string
		label                 string // for log messages
	}
	var allTokens []tokenCheck

	tokenLists := map[string][]*models.TokenInfo{
		constants.TokenType_RBT:           txnInfo.Tokens.RBT,
		constants.TokenType_FT:            txnInfo.Tokens.FT,
		constants.TokenType_NFT:           txnInfo.Tokens.NFT,
		constants.TokenType_SmartContract: txnInfo.Tokens.SmartContract,
	}
	for tokenType, tokens := range tokenLists {
		for _, t := range tokens {
			if t.PreviousTransactionID == "" {
				continue
			}
			allTokens = append(allTokens, tokenCheck{
				tokenID:               t.TokenID,
				previousTransactionID: t.PreviousTransactionID,
				syncFromPeerDID:       txnInfo.Initiator,
				label:                 tokenType,
			})
		}
	}

	// Include CommittedTokens (parent tokens being burnt/split). Without this,
	// the parent's prior chain history is never synced, and the upcoming chain
	// row insert can fail with a foreign-key violation on
	// previous_transaction_id when the prior tx is missing locally.
	for _, t := range txnInfo.CommittedTokens {
		if t == nil || t.TokenID == "" || t.PreviousTransactionID == "" {
			continue
		}
		allTokens = append(allTokens, tokenCheck{
			tokenID:               t.TokenID,
			previousTransactionID: t.PreviousTransactionID,
			syncFromPeerDID:       txnInfo.Initiator,
			label:                 "committed",
		})
	}

	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				if t.PreviousTransactionID == "" {
					continue
				}
				allTokens = append(allTokens, tokenCheck{
					tokenID:               t.TokenID,
					previousTransactionID: t.PreviousTransactionID,
					syncFromPeerDID:       quorum.Did,
					label:                 "pledge:" + quorum.Did,
				})
			}
		}
	}

	if len(allTokens) == 0 {
		return nil
	}

	// Phase 1: identify tokens whose local chain tip doesn't match.
	tokensToSyncByPeer := make(map[string][]string)
	tokenIDSyncPeerMap := make(map[string]string)
	tokenIDPrevTxIDMap := make(map[string]string)

	for _, tc := range allTokens {
		localTxID, err := w.GetLatestTransactionIdByTokenId(tc.tokenID, isFullnode)
		if err != nil {
			log.Debug("TokenChainIntigrityCheck: token not found locally, marking for sync",
				"tokenID", tc.tokenID, "label", tc.label)
			tokensToSyncByPeer[tc.syncFromPeerDID] = append(tokensToSyncByPeer[tc.syncFromPeerDID], tc.tokenID)
			tokenIDSyncPeerMap[tc.tokenID] = tc.syncFromPeerDID
			tokenIDPrevTxIDMap[tc.tokenID] = tc.previousTransactionID
			continue
		}
		if localTxID != tc.previousTransactionID {
			log.Debug("TokenChainIntigrityCheck: transaction ID mismatch, marking for sync",
				"tokenID", tc.tokenID,
				"localTxID", localTxID,
				"expectedPrevTxID", tc.previousTransactionID,
				"label", tc.label,
			)
			tokensToSyncByPeer[tc.syncFromPeerDID] = append(tokensToSyncByPeer[tc.syncFromPeerDID], tc.tokenID)
			tokenIDSyncPeerMap[tc.tokenID] = tc.syncFromPeerDID
			tokenIDPrevTxIDMap[tc.tokenID] = tc.previousTransactionID
		}
	}

	// Phase 2: sync tokens that are out of date.
	if len(tokensToSyncByPeer) > 0 && syncTxChains != nil {
		for peerDID, tokensToSync := range tokensToSyncByPeer {
			peerPrevTxIDs := make(map[string]string, len(tokensToSync))
			for _, tokenID := range tokensToSync {
				peerPrevTxIDs[tokenID] = tokenIDPrevTxIDMap[tokenID]
			}
			if err := syncTxChains(peerDID, tokensToSync, peerPrevTxIDs, []string{currentTxID}); err != nil {
				return fmt.Errorf("TokenChainIntigrityCheck: sync failed from %s: %w", peerDID, err)
			}
		}
	}

	// Phase 3: re-verify every token that was synced.
	for _, tc := range allTokens {
		if _, needsSync := tokenIDPrevTxIDMap[tc.tokenID]; !needsSync {
			continue // already matched in phase 1
		}

		localTxID, err := w.GetLatestTransactionIdByTokenId(tc.tokenID, isFullnode)
		if err != nil {
			return fmt.Errorf("TokenChainIntigrityCheck: token %s (%s) still not found locally after sync from %s",
				tc.tokenID, tc.label, tokenIDSyncPeerMap[tc.tokenID])
		}
		if localTxID != tc.previousTransactionID {
			return fmt.Errorf("TokenChainIntigrityCheck: token %s (%s) chain mismatch after sync from %s: local latest %s != expected %s",
				tc.tokenID, tc.label, tokenIDSyncPeerMap[tc.tokenID], localTxID, tc.previousTransactionID)
		}
	}

	return nil
}

// ValidateIPFSPinChecks validates IPFS pin status for all tokens.
// checkPinned is a caller-supplied function that checks whether a given
// token state hash is already pinned. It should return an error if pinned.
func ValidateIPFSPinChecks(txnInfo *models.TransactionInfo, isFullnode bool, checkPinned func(tokenID, previousTxnID string) error) error {
	if txnInfo.Tokens == nil {
		return nil
	}

	tokenLists := [][]*models.TokenInfo{
		txnInfo.Tokens.RBT,
		txnInfo.Tokens.FT,
		txnInfo.Tokens.NFT,
		txnInfo.Tokens.SmartContract,
	}

	for _, tokens := range tokenLists {
		for _, t := range tokens {
			if err := checkPinned(t.TokenID, t.PreviousTransactionID); err != nil {
				return err
			}
		}
	}

	for _, t := range txnInfo.CommittedTokens {
		if err := checkPinned(t.TokenID, t.PreviousTransactionID); err != nil {
			return err
		}
	}

	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				if err := checkPinned(t.TokenID, t.PreviousTransactionID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ValidateTransaction orchestrates all validation checks on an incoming transaction.
//
// Parameters that replace Core method calls:
//   - w: wallet for DB lookups
//   - log: logger
//   - initiatorDC: pre-initialised DIDCrypto for the transaction initiator
//   - quorumDCs: map of quorum DID → pre-initialised DIDCrypto
//   - testnet, mainnet, localnet: network mode flags
//   - checkPinned: function to check IPFS pin status (tokenID, prevTxnID) → error
//   - syncTxChains: callback to sync transaction chains from peer (injected from core to avoid cyclic import)
func ValidateTransaction(
	tx *models.Transactions,
	isFullnode bool,
	w *wallet.Wallet,
	log logger.Logger,
	initiatorDC types.DIDCrypto,
	quorumDCs map[string]types.DIDCrypto,
	testnet bool,
	mainnet bool,
	localnet bool,
	checkPinned func(tokenID, previousTxnID string) error,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
) (bool, error) {
	var txnInfo models.TransactionInfo
	if err := json.Unmarshal(tx.Info, &txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: failed to unmarshal transaction info: %w", err)
	}

	if err := ValidateTransactionInfoFields(&txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	if err := TransactionIDIntegrityCheck(tx.ID, &txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	var sig models.Signature
	if err := json.Unmarshal(tx.Signature, &sig); err != nil {
		return false, fmt.Errorf("ValidateTransaction: failed to unmarshal signature: %w", err)
	}

	if err := SignatureVerificationCheck(&txnInfo, &sig, initiatorDC, quorumDCs); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	if err := TokenChainIntegrityCheck(&txnInfo, tx.ID, isFullnode, w, log, syncTxChains); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	if err := ValidateTokenOwnershipByPrevTxn(&txnInfo, isFullnode, w); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	// 7. ValidateTokenIDRelatedChecks for each RBT token in Tokens and CommittedTokens and pledged tokens
	if txnInfo.Tokens.RBT != nil {
		for _, token := range txnInfo.Tokens.RBT {
			if err := ValidateTokenIDRelatedChecks(token.TokenID, txnInfo.Initiator, isFullnode, w, testnet, mainnet, localnet, log, syncTxChains); err != nil {
				return false, fmt.Errorf("ValidateTransaction: token %s: %w", token.TokenID, err)

			}
		}
	}

	for _, t := range txnInfo.CommittedTokens {
		if err := ValidateTokenIDRelatedChecks(t.TokenID, txnInfo.Initiator, isFullnode, w, testnet, mainnet, localnet, log, syncTxChains); err != nil {
			return false, fmt.Errorf("ValidateTransaction: committed token %s: %w", t.TokenID, err)
		}
	}

	//Both Fullnode as well as quorum will do the ValidateTokenIDRelatedChecks checks for pledged tokens

	for _, quorum := range txnInfo.Quorums {
		for _, t := range quorum.Tokens {
			if err := ValidateTokenIDRelatedChecks(t.TokenID, quorum.Did, isFullnode, w, testnet, mainnet, localnet, log, syncTxChains); err != nil {
				return false, fmt.Errorf("ValidateTransaction: quorum %s token %s: %w", quorum.Did, t.TokenID, err)

			}
		}
	}

	//TransactionValue and Pledge value will not match incase of minting transaction
	if isFullnode {
		if err := ValidateTransactionValueAndPledge(&txnInfo); err != nil {
			return false, fmt.Errorf("ValidateTransaction: %w", err)
		}
	}

	if err := ValidateIPFSPinChecks(&txnInfo, isFullnode, checkPinned); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	return true, nil
}

// GetPreviousTxnIDForToken returns the PreviousTransactionID for the given
// tokenID by scanning the token lists inside txInfo. It searches in the
// following order: Tokens.RBT, Tokens.NFT, Tokens.FT, Tokens.SmartContract,
// The second return value indicates whether the tokenID was found.
func GetPreviousTxnIDForToken(tokenID string, txInfo *models.TransactionInfo) (string, bool) {
	if txInfo == nil {
		return "", false
	}

	if txInfo.Tokens != nil {
		for _, list := range [][]*models.TokenInfo{
			txInfo.Tokens.RBT,
			txInfo.Tokens.NFT,
			txInfo.Tokens.FT,
			txInfo.Tokens.SmartContract,
		} {
			for _, t := range list {
				if t != nil && t.TokenID == tokenID {
					return t.PreviousTransactionID, true
				}
			}
		}
	}

	return "", false
}

// isTokenPledged returns true when tokenID is present in quorum pledge tokens.
func isTokenPledged(txInfo *models.TransactionInfo, tokenID string) bool {
	if txInfo == nil {
		return false
	}

	for _, quorum := range txInfo.Quorums {
		if quorum == nil {
			continue
		}
		for _, token := range quorum.Tokens {
			if token != nil && token.TokenID == tokenID {
				return true
			}
		}
	}

	return false
}

// getTransactionTokens returns true if at least one token present in pledgedTxnInfo.Tokens
// is also present in unpledgeTxnInfo transfer tokens, committed tokens, or quorum tokens.
func getTransactionTokens(pledgedTxnInfo *models.TransactionInfo, unpledgeTxnInfo *models.TransactionInfo) bool {
	if pledgedTxnInfo == nil || pledgedTxnInfo.Tokens == nil || unpledgeTxnInfo == nil {
		return false
	}

	prevTxnTokenIDs := make(map[string]struct{})

	if unpledgeTxnInfo.Tokens != nil {
		for _, list := range [][]*models.TokenInfo{
			unpledgeTxnInfo.Tokens.RBT,
			unpledgeTxnInfo.Tokens.NFT,
			unpledgeTxnInfo.Tokens.FT,
			unpledgeTxnInfo.Tokens.SmartContract,
		} {
			for _, token := range list {
				if token != nil && token.TokenID != "" {
					prevTxnTokenIDs[token.TokenID] = struct{}{}
				}
			}
		}
	}

	for _, token := range unpledgeTxnInfo.CommittedTokens {
		if token != nil && token.TokenID != "" {
			prevTxnTokenIDs[token.TokenID] = struct{}{}
		}
	}

	for _, quorum := range unpledgeTxnInfo.Quorums {
		if quorum == nil {
			continue
		}
		for _, token := range quorum.Tokens {
			if token != nil && token.TokenID != "" {
				prevTxnTokenIDs[token.TokenID] = struct{}{}
			}
		}
	}

	for _, list := range [][]*models.TokenInfo{
		pledgedTxnInfo.Tokens.RBT,
		pledgedTxnInfo.Tokens.NFT,
		pledgedTxnInfo.Tokens.FT,
		pledgedTxnInfo.Tokens.SmartContract,
	} {
		for _, token := range list {
			if token == nil || token.TokenID == "" {
				continue
			}
			if _, exists := prevTxnTokenIDs[token.TokenID]; exists {
				return true
			}
		}
	}

	return false
}
