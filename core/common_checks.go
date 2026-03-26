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

	if txnInfo.Tokens == nil ||
		(len(txnInfo.Tokens.RBT) == 0 && len(txnInfo.Tokens.NFT) == 0 &&
			len(txnInfo.Tokens.FT) == 0 && len(txnInfo.Tokens.SmartContract) == 0) {
		return fmt.Errorf("transaction must contain at least one transfer token (RBT, NFT, FT, or SmartContract)")
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

// In this function, we are validating the initiator & quorum signatures.
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
func (c *Core) ValidateNewTokenContent(tokenID string, isQuorum bool) error {
	devidedParts := strings.Split(tokenID, "_")

	tokenTypeString := RBTString
	if len(devidedParts) == 3 {
		tokenTypeString = PartString
	}

	// parse level (e.g. "002")
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}

	// parse token number (e.g. "1000")
	tokenNo, err := strconv.Atoi(devidedParts[1])
	if err != nil {
		return fmt.Errorf("invalid token number in token content: %s", tokenID)
	}
	
	shouldValidate := c.testnet || c.mainnet || (c.localnet && isQuorum) //pass 

	if shouldValidate {
		mapLevel := level
		network := "mainnet"

		if c.testnet {
			network = "testnet"
			if level < constants.FaucetRBT_Level_Offset {
				return fmt.Errorf(
					"invalid testnet token level %d: testnet level must be >= %d",
					level, constants.FaucetRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.FaucetRBT_Level_Offset
		} else if c.localnet {
			network = "localnet"
			if level < constants.LocalRBT_Level {
				return fmt.Errorf(
					"invalid local token level %d: localnet level must be > %d",
					level, constants.LocalRBT_Level,
				)
			}
			mapLevel = level - constants.LocalRBT_Level
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
		c.log.Debug("token content validated for "+network, tokenID)
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
		c.log.Debug("token content validated for the part token", tokenID)

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
	var err error
	var tokenDetails models.FullNodeRBT

	if isFullNode {
		tokenDetails, err = c.w.GetFullNodeRBTToken(tokenID)
		if err == nil {
			if !tokenDetails.ParentTokenID.Valid || tokenDetails.ParentTokenID.String == "" {
				//instead of return, it should compute parent tokenID from the tokenID
				// TODO: replace with proper parent tokenID computation once available
				partTokenID := parts.TokenID(tokenID)
				parentTokenID, err := partTokenID.GetParentToken()
				if err != nil {
					return fmt.Errorf("failed to get parent for token %s: %w", partTokenID, err), false
				}
				if parentTokenID == "" {
					return nil, false
				}

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
		}
	}
	var genesisTx *models.Transactions
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

func (c *Core) ValidateTokenIDRelatedChecks(tokenID string, isFullNode bool) error {
	//1. call ValidateNewTokenContent
	err := c.ValidateNewTokenContent(tokenID, isFullNode)
	if err != nil {
		return fmt.Errorf("failed to validate token content: %w", err)
	}
	//2. call IsParentTokenBurnt
	err, isParentTokenBurnt := c.IsParentTokenBurnt(isFullNode, tokenID)
	if err != nil {
		return fmt.Errorf("failed to validate parent token burnt: %w", err)
	}
	if !isParentTokenBurnt {
		return fmt.Errorf("parent token is not burnt")
	}
	//3. call genuine token creator check
	err = c.ValidateGenuineTokenCreator(tokenID, isFullNode)
	if err != nil {
		return fmt.Errorf("failed to validate genuine token creator: %w", err)
	}
	return nil
}

// This function is used to validate the genuinity of the token creator.Currently it is a placeholder function.needs to complete it.
func (c *Core) ValidateGenuineTokenCreator(tokenID string, isFullNode bool) error {
	// Check the level number and see if its Level 1. If so, then it is part of the Premint token series. Here we:
	// We get the genesis transaction from the tokenchain table
	// Then use that TransactionID to query the transactionInfo from transactions table.
	// We get the owner from transaction table.
	devidedParts := strings.Split(tokenID, "_")

	// parse level (e.g. "002")
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}
	if level == 1 {
		//We get the genesis transaction from the tokenchain table
		genesisTx, err := c.w.GetGenesisTransactionIdByTokenId(tokenID, isFullNode)
		if err != nil {
			return fmt.Errorf("failed to get genesis transaction for token %s: %w", tokenID, err)
		}
		genesisTxInfo, err := c.w.GetTransactionByID(genesisTx)
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

// TokenChainIntigrityCheck checks each token in the TransactionInfo against the
// tokens table. If the PreviousTransactionID in the incoming transaction does
// not match the latest TransactionID stored locally, it triggers a chain sync
// for that token via syncTransactionChainFrom.
// Then it will check that whether previous transaction of the incoming transaction is same as the latest transaction in the token chain.
// Then it will check the role of the token in the previous transaction. If the role is pledge, or burnt, or commit then it will return nil,false.
func (c *Core) TokenChainIntigrityCheck(txnInfo *models.TransactionInfo) (error, bool) {
	if txnInfo.Tokens == nil {
		return nil, false
	}

	tokenLists := [][]*models.TokenInfo{
		txnInfo.Tokens.RBT,
		txnInfo.Tokens.FT,
		txnInfo.Tokens.NFT,
		txnInfo.Tokens.SmartContract,
	}

	for _, tokens := range tokenLists {
		for _, t := range tokens {
			tokenDetails, err := c.w.GetTokenByTokenID(t.TokenID)
			if err != nil {
				c.log.Debug("token not found locally, syncing full chain", "tokenID", t.TokenID)
				peer, err := c.getPeer(txnInfo.Initiator)
				if err != nil {
					c.log.Error("InitiateTransaction: Failed to get peer for receiver", "err", err)
				}
				defer peer.Close()
				if syncErr, _ := c.syncTransactionChainFrom(peer, t.PreviousTransactionID, t.TokenID); syncErr != nil {
					return fmt.Errorf("failed to sync token chain for %s: %w", t.TokenID, syncErr), false
				}
				continue
			}

			if tokenDetails.TransactionID != t.PreviousTransactionID {
				c.log.Debug("transaction ID mismatch, syncing chain",
					"tokenID", t.TokenID,
					"existingTransactionID", tokenDetails.TransactionID,
					"incomingTransactionID", t.PreviousTransactionID,
				)
			}
			if tokenDetails.TransactionID != t.PreviousTransactionID {
				return fmt.Errorf("transaction ID mismatch for token %s: %s != %s", t.TokenID, tokenDetails.TransactionID, t.PreviousTransactionID), false
			}
			previousTransaction, err := c.w.GetTransactionByID(t.PreviousTransactionID)
			if err != nil {
				return fmt.Errorf("failed to get previous transaction for token %s: %w", t.TokenID, err), false
			}
			if previousTransaction == nil {
				return fmt.Errorf("previous transaction not found for token %s", t.TokenID), false
			}
			var previousTransactionInfo models.TransactionInfo
			if err := json.Unmarshal(previousTransaction.Info, &previousTransactionInfo); err != nil {
				return fmt.Errorf("failed to unmarshal previous transaction info for token %s: %w", t.TokenID, err), false
			}
			//get role of the token in the transaction
			role := findTokenRoleInTxn(t.TokenID, &previousTransactionInfo)
			if role == int16(models.GetTokenRoleID(constants.TokenRole_Pledge)) ||
				role == int16(models.GetTokenRoleID(constants.TokenRole_Burn)) ||
				role == int16(models.GetTokenRoleID(constants.TokenRole_Commit)) {
				return nil, false
			}
		}
	}

	return nil, true
}

func (c *Core) ValidateIPFSPinChecks(txnInfo *models.TransactionInfo, isFullnode bool) error {
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
			if err := c.checkTokenStateHashPinned(t.TokenID, t.PreviousTransactionID); err != nil {
				return err
			}
		}
	}

	for _, t := range txnInfo.CommittedTokens {
		if err := c.checkTokenStateHashPinned(t.TokenID, t.PreviousTransactionID); err != nil {
			return err
		}
	}

	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				if err := c.checkTokenStateHashPinned(t.TokenID, t.PreviousTransactionID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (c *Core) checkTokenStateHashPinned(tokenID string, previousTransactionID string) error {
	if previousTransactionID == "" {
		return nil
	}

	tokenStateHash := tokenID + "." + previousTransactionID

	record, err := c.ipfsProviderStore.GetProviderByCID(tokenStateHash)
	if err != nil {
		return fmt.Errorf("failed to check pin status for %s: %w", tokenStateHash, err)
	}
	if record != nil {
		return fmt.Errorf("token %s is already pinned", tokenStateHash)
	}

	return nil
}

// ValidateTransaction orchestrates all validation checks on an incoming transaction.
func (c *Core) ValidateTransaction(tx *models.Transactions, isFullnode bool) (bool, error) {
	// 1. Unmarshal the transaction info
	var txnInfo models.TransactionInfo
	if err := json.Unmarshal(tx.Info, &txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: failed to unmarshal transaction info: %w", err)
	}

	// 2. Validate TransactionInfo fields (DID format, epoch, network, token lists)
	if err := c.ValidateTransactionInfoFields(&txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	// 3. TransactionID integrity check
	if err := c.TransactionIDIntegrityCheck(tx.ID, &txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	// 4. Signature verification
	if err := c.SignatureVerificationCheck(tx); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	// 5. Validate token ownership by previous transaction
	if err := c.ValidateTokenOwnershipByPrevTxn(&txnInfo); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	// 6. Sync token chains if needed
	if syncErr, ok := c.TokenChainIntigrityCheck(&txnInfo); syncErr != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", syncErr)
	} else if !ok {
		return false, fmt.Errorf("ValidateTransaction: token chain sync failed")
	}

	// 7. ValidateTokenIDRelatedChecks for each token in Tokens and CommittedTokens
	if txnInfo.Tokens != nil {
		for _, tokenList := range [][]*models.TokenInfo{
			txnInfo.Tokens.RBT, txnInfo.Tokens.FT,
			txnInfo.Tokens.NFT, txnInfo.Tokens.SmartContract,
		} {
			for _, t := range tokenList {
				if err := c.ValidateTokenIDRelatedChecks(t.TokenID, isFullnode); err != nil {
					return false, fmt.Errorf("ValidateTransaction: token %s: %w", t.TokenID, err)
				}
			}
		}
	}

	for _, t := range txnInfo.CommittedTokens {
		if err := c.ValidateTokenIDRelatedChecks(t.TokenID, isFullnode); err != nil {
			return false, fmt.Errorf("ValidateTransaction: committed token %s: %w", t.TokenID, err)
		}
	}

	// If isFullNode, also validate tokens in each Quorum
	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				if err := c.ValidateTokenIDRelatedChecks(t.TokenID, isFullnode); err != nil {
					return false, fmt.Errorf("ValidateTransaction: quorum %s token %s: %w", quorum.Did, t.TokenID, err)
				}
			}
		}
	}
	//8. If isFullnode, validate the transaction value and pledge
	if isFullnode {
		if err := c.ValidateTransactionValueAndPledge(&txnInfo); err != nil {
			return false, fmt.Errorf("ValidateTransaction: %w", err)
		}
	}
	//9.add the IPFS pin checks here.
	if err := c.ValidateIPFSPinChecks(&txnInfo, isFullnode); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	return true, nil
}
