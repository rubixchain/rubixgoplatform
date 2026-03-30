package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) ValidateOwnershipOfTheToken(assetType int, tokenID string, initiator string) error {
	if assetType == RBTTokenType || assetType == FTTokenType || assetType == NFTTokenType {
		//TODO: fetch owner of the previous transaction from the token chain
		// owner, err := c.w.GetOwnerOfTheToken(tokenID)
		// if err != nil {
		// 	return fmt.Errorf("failed to get owner of the token: %v", err)
		// }
		// if owner != initiator {
		// 	return fmt.Errorf("owner of the token is not the initiator")
		// }
	}
	return nil
}

// ValidateTokenOwnershipByPrevTxn groups all RBT, FT and NFT tokens by their
// PreviousTransactionID, fetches each distinct previous transaction once, and
// verifies that its owner matches the current transaction's initiator.
func (c *Core) ValidateTokenOwnershipByPrevTxn(txnInfo *models.TransactionInfo) error {
	if txnInfo.Tokens == nil {
		return nil
	}

	prevTxnTokensMap := make(map[string][]string)

	tokenLists := [][]*models.TokenInfo{
		txnInfo.Tokens.RBT,
		txnInfo.Tokens.NFT,
		txnInfo.Tokens.FT,
	}

	for _, tokens := range tokenLists {
		for _, t := range tokens {
			if t.PreviousTransactionID == "" {
				continue
			}
			prevTxnTokensMap[t.PreviousTransactionID] = append(
				prevTxnTokensMap[t.PreviousTransactionID], t.TokenID,
			)
		}
	}

	for prevTxnID, tokenIDs := range prevTxnTokensMap {
		prevTx, err := c.w.GetTransactionByID(prevTxnID)
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

		if prevTxnInfo.Owner != txnInfo.Initiator {
			return fmt.Errorf(
				"ownership mismatch: initiator %s does not match owner %s of previous transaction %s (affected tokens: %v)",
				txnInfo.Initiator, prevTxnInfo.Owner, prevTxnID, tokenIDs,
			)
		}
	}

	return nil
}

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

// In this function, we are validating the Initiator & Owner's DID validations;
// Epoch, Network, Tokens, CommittedTokens, Quorums fields of the transaction info.
func (c *Core) ValidateTransactionInfoFields(txnInfo *models.TransactionInfo) error {
	if err := validateDID(txnInfo.Initiator, "initiator"); err != nil {
		return err
	}

	if err := validateDID(txnInfo.Owner, "owner"); err != nil {
		return err
	}

	currentEpoch := int(time.Now().Unix())
	if txnInfo.Epoch <= 0 || txnInfo.Epoch > currentEpoch {
		return fmt.Errorf("invalid epoch %d: must be a positive integer less than current epoch %d", txnInfo.Epoch, currentEpoch)
	}

	if _, ok := validNetworks[txnInfo.Network]; !ok {
		return fmt.Errorf("invalid network %q: must be one of the supported networks", txnInfo.Network)
	}

	if txnInfo.Tokens != nil {
		tokenLists := map[string][]*models.TokenInfo{
			"RBT":           txnInfo.Tokens.RBT,
			"NFT":           txnInfo.Tokens.NFT,
			"FT":            txnInfo.Tokens.FT,
			"SmartContract": txnInfo.Tokens.SmartContract,
		}
		for listName, tokens := range tokenLists {
			if err := validateTokenInfoList(tokens, "tokens."+listName); err != nil {
				return err
			}
		}
	}

	if err := validateTokenInfoList(txnInfo.CommittedTokens, "committedTokens"); err != nil {
		return err
	}

	for i, quorum := range txnInfo.Quorums {
		if err := validateDID(quorum.Did, fmt.Sprintf("quorum[%d]", i)); err != nil {
			return err
		}
		if err := validateTokenInfoList(quorum.Tokens, fmt.Sprintf("quorums[%d].tokens", i)); err != nil {
			return err
		}
	}

	return nil
}

func validateTokenInfoList(tokens []*models.TokenInfo, context string) error {
	for i, t := range tokens {
		//CommittedTokens list might be empty, In case if there are no burnt tokens or some kind of cases.
		if !contains(context, "committedTokens") && t == nil {
			return fmt.Errorf("nil token entry at %s[%d]", context, i)
		}
		if t.TokenID == "" {
			return fmt.Errorf("empty tokenID at %s[%d]", context, i)
		}

	}
	return nil
}

func (c *Core) TransactionIDIntegrityCheck(transactionID string, transactionInfo *models.TransactionInfo) error {

	computedTransactionID, err := util.GetTransactionID(transactionInfo)
	if err != nil {
		return fmt.Errorf("failed to compute transaction ID: %w", err)
	}
	if computedTransactionID != transactionID {
		return fmt.Errorf("transaction ID mismatch: computed %s != provided %s", computedTransactionID, transactionID)
	}
	return nil

}

func (c *Core) SignatureVerificationCheck(tx *models.Transactions) error {
	var txInfo models.TransactionInfo
	if err := json.Unmarshal(tx.Info, &txInfo); err != nil {
		return fmt.Errorf("SignatureVerificationCheck: failed to unmarshal transaction info: %w", err)
	}

	var sig models.Signature
	if err := json.Unmarshal(tx.Signature, &sig); err != nil {
		return fmt.Errorf("SignatureVerificationCheck: failed to unmarshal signature: %w", err)
	}

	initiatorDC, err := c.InitialiseDID(txInfo.Initiator)
	if err != nil {
		return fmt.Errorf("SignatureVerificationCheck: failed to initialise initiator DID %s: %w", txInfo.Initiator, err)
	}

	if err := util.VerifySignature(initiatorDC, &txInfo, sig.InitiatorSignature); err != nil {
		return fmt.Errorf("SignatureVerificationCheck: initiator signature verification failed: %w", err)
	}
	if len(sig.Quorums) != 0 {
		for _, quorumSig := range sig.Quorums {
			quorumDC, err := c.InitialiseDID(quorumSig.Did)
			if err != nil {
				return fmt.Errorf("SignatureVerificationCheck: failed to initialise quorum DID %s: %w", quorumSig.Did, err)
			}

			if err := util.VerifySignature(quorumDC, &txInfo, quorumSig.Signature); err != nil {
				return fmt.Errorf("SignatureVerificationCheck: quorum %s signature verification failed: %w", quorumSig.Did, err)
			}
		}
	}

	return nil
}

// In this function, we are validating the token number and level for the new token content.
// For localnet tokens, only the quorum should validate the token number and level.
func (c *Core) ValidateNewTokenContent(tokenContent string, isQuorum bool) error {
	devidedParts := strings.Split(tokenContent, "_")

	tokenTypeString := RBTString
	if len(devidedParts) == 3 {
		tokenTypeString = PartString
	}

	// parse level (e.g. "002")
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenContent)
	}

	// parse token number (e.g. "1000")
	tokenNo, err := strconv.Atoi(devidedParts[1])
	if err != nil {
		return fmt.Errorf("invalid token number in token content: %s", tokenContent)
	}

	shouldValidate := c.testnet || c.mainnet || (c.localnet && isQuorum)

	if shouldValidate {
		mapLevel := level
		network := "mainnet"

		if c.testnet {
			network = "testnet"
			if level < constants.TestnetRBT_Level_Offset {
				return fmt.Errorf(
					"invalid testnet token level %d: testnet level must be >= %d",
					level, constants.TestnetRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.TestnetRBT_Level_Offset
		} else if c.localnet {
			network = "localnet"
			if level <= constants.LocalRBT_Level_Offset {
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
		c.log.Debug("token content validated for "+network, tokenContent)
	}

	MaxPossiblePartTokenNumber := parts.MaxPossiblePartsIndexByMaxDecimalPlaces(uint(constants.MaxSupportedDecimalPlaces))
	if tokenTypeString == PartString {
		partTokenNumber, err := strconv.Atoi(devidedParts[2])
		if err != nil {
			return fmt.Errorf("invalid part number in token content: %s", tokenContent)
		}
		if partTokenNumber > MaxPossiblePartTokenNumber {
			return fmt.Errorf(
				"Parttoken number %d exceeds max allowed %d ",
				partTokenNumber, MaxPossiblePartTokenNumber,
			)

		}
		c.log.Debug("token content validated for the part token", tokenContent)

	}

	return nil
}

// ValidateTransactionValueAndPledge checks that the total value of RBT tokens
// in the transaction equals the total value of tokens pledged across all quorums.FullNode will do this check.
func (c *Core) ValidateTransactionValueAndPledge(txnInfo *models.TransactionInfo) error {
	if txnInfo.Tokens == nil {
		return fmt.Errorf("transaction has no tokens")
	}

	transactionValue := rubixmath.ZeroFloat()
	for _, t := range txnInfo.Tokens.RBT {
		tokenValue, err := util.GetTokenValueFromTokenID(t.TokenID)
		if err != nil {
			return fmt.Errorf("failed to get value for RBT token %s: %w", t.TokenID, err)
		}
		transactionValue = rubixmath.AddFloat(transactionValue, tokenValue)
	}

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

	if transactionValue != totalPledgeValue {
		return fmt.Errorf(
			"transaction value (%v) does not match total pledge amount (%v)",
			transactionValue, totalPledgeValue,
		)
	}

	return nil
}

func (c *Core) IsParentTokenBurnt(isFullNode bool, tokenID string) (error, bool) {
	// TODO(phase11): IsParentTokenBurnt had a broken brace structure in the upstream.
	// Rewritten as a clean stub pending reimplementation.
	var parentTokenID string

	if isFullNode {
		tokenDetails, err := c.w.GetFullNodeRBTToken(tokenID)
		if err == nil {
			if !tokenDetails.ParentTokenID.Valid || tokenDetails.ParentTokenID.String == "" {
				// TODO: replace with proper parent tokenID computation once available
				partTokenID := parts.TokenID(tokenID)
				computedParent, err := partTokenID.GetParentToken()
				if err != nil {
					return fmt.Errorf("failed to get parent for token %s: %w", partTokenID, err), false
				}
				if computedParent == "" {
					return nil, false
				}
				parentTokenID = computedParent
			} else {
				parentTokenID = tokenDetails.ParentTokenID.String
			}
		} else {
			// TODO: replace with proper parent tokenID computation once available
			partTokenID := parts.TokenID(tokenID)
			computedParent, err := partTokenID.GetParentToken()
			if err != nil {
				return fmt.Errorf("failed to get parent id of token %s: %w", partTokenID, err), false
			}
			if computedParent == "" {
				return nil, false
			}
			parentTokenID = computedParent
		}
	} else {
		// TODO: replace with proper parent tokenID computation once available
		partTokenID := parts.TokenID(tokenID)
		computedParent, err := partTokenID.GetParentToken()
		if err != nil {
			return fmt.Errorf("failed to get parent id of token %s: %w", partTokenID, err), false
		}
		if computedParent == "" {
			return nil, false
		}
		parentTokenID = computedParent
	}

	var genesisTx *models.Transactions
	var err error
	if isFullNode {
		genesisTx, _, err = c.w.GetFullNodeTransactionAndRoleAtHeight(tokenID, 0)
	} else {
		genesisTx, _, err = c.w.GetTransactionAndRoleAtHeight(tokenID, 0)
	}
	if err != nil {
		return fmt.Errorf("failed to get genesis transaction for token %s: %w", tokenID, err), false
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

// In this function, all the validation related functions will get called inside this function.
func (c *Core) ValidateTransaction(tx *models.Transactions, isFullnode bool) (bool, error) {

	return true, nil
}
