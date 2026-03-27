package consensus

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
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

	APISyncTransactionChain string = "/api/sync-transaction-chain"
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
		prevTx, err := w.GetTransactionByID(prevTxnID)
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

	//if isFullnode, Fullnode checks that owner of the previous transaction is same as the quorum which has pledged this token
	if isFullnode {
		for _, quorum := range txnInfo.Quorums {
			for _, t := range quorum.Tokens {
				tokenDetails, err := w.GetFullNodeRBTToken(t.TokenID)
				if err != nil {
					return fmt.Errorf("failed to get fullnode RBT token %s: %w", t.TokenID, err)
				} //TODO: Handle the case where there is no RBT token in the fullnode rbt tokens table
				previousTransactionOwner := tokenDetails.DID
				if previousTransactionOwner != quorum.Did {
					return fmt.Errorf("ownership mismatch: quorum %s does not match owner %s of previous transaction %s (affected tokens: %v)", quorum.Did, tokenDetails.DID, t.TokenID)
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

	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}

	tokenNo, err := strconv.Atoi(devidedParts[1])
	if err != nil {
		return fmt.Errorf("invalid token number in token content: %s", tokenID)
	}

	shouldValidate := testnet || mainnet || (localnet && isQuorum)

	if shouldValidate {
		mapLevel := level
		network := "mainnet"

		if testnet {
			network = "testnet"
			if level < constants.FaucetRBT_Level_Offset {
				return fmt.Errorf(
					"invalid testnet token level %d: testnet level must be >= %d",
					level, constants.FaucetRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.FaucetRBT_Level_Offset
		} else if localnet {
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
		log.Debug("token content validated for "+network, tokenID)
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
		log.Debug("token content validated for the part token", tokenID)
	}

	return nil
}

func ValidateTransactionValueAndPledge(txnInfo *models.TransactionInfo) error {
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

func IsParentTokenBurnt(isFullNode bool, tokenID string, w *wallet.Wallet) (error, bool) {
	var parentTokenID string
	var err error
	var tokenDetails models.FullNodeRBT

	if isFullNode {
		tokenDetails, err = w.GetFullNodeRBTToken(tokenID)
		if err == nil {
			if !tokenDetails.ParentTokenID.Valid || tokenDetails.ParentTokenID.String == "" {
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
		genesisTx, _, err = w.GetFullNodeTransactionAndRoleAtHeight(tokenID, 0)
	} else {
		genesisTx, _, err = w.GetTransactionAndRoleAtHeight(tokenID, 0)
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

func ValidateGenuineTokenCreator(tokenID string, isFullNode bool, w *wallet.Wallet) error {
	devidedParts := strings.Split(tokenID, "_")

	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenID)
	}
	if level == 1 {
		genesisTx, err := w.GetGenesisTransactionIdByTokenId(tokenID, isFullNode)
		if err != nil {
			return fmt.Errorf("failed to get genesis transaction for token %s: %w", tokenID, err)
		}
		genesisTxInfo, err := w.GetTransactionByID(genesisTx)
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

func ValidateTokenIDRelatedChecks(tokenID string, isFullNode bool, w *wallet.Wallet, testnet bool, mainnet bool, localnet bool, log logger.Logger) error {
	err := ValidateNewTokenContent(tokenID, isFullNode, testnet, mainnet, localnet, log)
	if err != nil {
		return fmt.Errorf("failed to validate token content: %w", err)
	}
	//call IsParentTokenBurnt
	//First check whether the token is a part token or not. If it is a part token, then check whether the parent token is burnt.
	devidedParts := strings.Split(tokenID, "_")
	if len(devidedParts) == 3 {
		err, isParentTokenBurnt := IsParentTokenBurnt(isFullNode, tokenID, w)
		if err != nil {
			return fmt.Errorf("failed to validate parent token burnt: %w", err)
		}
		if !isParentTokenBurnt {
			return fmt.Errorf("parent token is not burnt")
		}
	}
	err = ValidateGenuineTokenCreator(tokenID, isFullNode, w)
	if err != nil {
		return fmt.Errorf("failed to validate genuine token creator: %w", err)
	}
	return nil
}

func FindTokenRoleInTxn(tokenID string, txInfo *models.TransactionInfo) int16 {
	if txInfo.Tokens != nil {
		for _, lists := range [][]*models.TokenInfo{
			txInfo.Tokens.RBT, txInfo.Tokens.NFT,
			txInfo.Tokens.FT, txInfo.Tokens.SmartContract,
		} {
			for _, t := range lists {
				if t.TokenID == tokenID {
					return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
				}
			}
		}
	}

	for _, t := range txInfo.CommittedTokens {
		if t.TokenID == tokenID {
			return int16(models.GetTokenRoleID(constants.TokenRole_Commit))
		}
	}

	for _, q := range txInfo.Quorums {
		for _, t := range q.Tokens {
			if t.TokenID == tokenID {
				return int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
			}
		}
	}

	return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
}

func SyncTransactionChainFrom(p *ipfsport.Peer, tokenID string, w *wallet.Wallet, log logger.Logger) (error, *models.TransactionChainSyncReply) {
	var err error

	latestTransactionID := w.GetLatestTransactionID(tokenID)
	if latestTransactionID == "" {
		log.Error("failed to get latest transaction id")
		return err, nil
	}

	syncReq := models.TransactionChainSyncRequest{
		TokenID:       tokenID,
		TransactionID: latestTransactionID,
	}

	for {
		var trep models.TransactionChainSyncReply
		err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &syncReq, &trep, false)
		if err != nil {
			log.Error("failed to sync transaction chain")
			return err, nil
		}
		if !trep.Status {
			log.Error("failed to sync transaction chain")
			return fmt.Errorf(trep.Message), nil
		}
		if len(trep.Transactions) > 0 {
			for _, txn := range trep.Transactions {
				tx, err := util.TransactionFromBytes(txn)
				if tx == nil {
					log.Error("failed to convert transaction bytes to transaction")
					return fmt.Errorf("failed to convert transaction bytes to transaction"), nil
				}
				var txInfo models.TransactionInfo
				if err = json.Unmarshal(tx.Info, &txInfo); err != nil {
					log.Error("failed to unmarshal transaction info", "err", err)
					return fmt.Errorf("failed to unmarshal transaction info: %w", err), nil
				}

				role := FindTokenRoleInTxn(tokenID, &txInfo)

				if err = w.CreateTransaction(tx); err != nil {
					log.Error("failed to add transaction to transactions table", "err", err)
					return fmt.Errorf("failed to add transaction: %w", err), nil
				}

				tokenDetails, err := w.GetTokenByTokenID(tokenID)
				if err != nil {
					newToken := models.Token{
						TokenID:        tokenID,
						TokenStatus:    constants.TokenStatus_Free,
						DID:            txInfo.Owner,
						TransactionID:  tx.ID,
						TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
						LatestPosition: 0,
						LatestRole:     role,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}
					if createErr := w.CreateRBTToken(newToken); createErr != nil {
						log.Error("failed to create token", "err", createErr)
						return fmt.Errorf("failed to create token: %w", createErr), nil
					}
					tokenDetails = newToken
				} else {
					tokenDetails.DID = txInfo.Owner
					tokenDetails.TransactionID = tx.ID
					tokenDetails.LatestPosition++
					tokenDetails.LatestRole = role
					if updateErr := w.UpdateToken(tokenDetails); updateErr != nil {
						log.Error("failed to update token", "err", updateErr)
						return fmt.Errorf("failed to update token: %w", updateErr), nil
					}
				}

				entry := &models.TokenChain{
					TokenID:       tokenID,
					TransactionID: tx.ID,
					Role:          role,
					Position:      tokenDetails.LatestPosition,
				}
				if err = w.AddTokenChainEntry(entry); err != nil {
					log.Error("failed to add token chain entry", "err", err)
					return fmt.Errorf("failed to add token chain entry: %w", err), nil
				}
			}
		}
		if trep.NextTransactionID == "" {
			break
		}
		syncReq.TransactionID = trep.NextTransactionID
	}
	return nil, nil
}

// TokenChainIntigrityCheck verifies each token's chain integrity.
// peer is the pre-resolved peer for the initiator (caller obtains via getPeer).
func TokenChainIntigrityCheck(txnInfo *models.TransactionInfo, peer *ipfsport.Peer, isFullnode bool, w *wallet.Wallet, log logger.Logger) (error, bool) {
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
			tokenDetails, err := w.GetTokenByTokenID(t.TokenID)
			if err != nil {
				log.Debug("token not found locally, syncing full chain", "tokenID", t.TokenID)
				if syncErr, _ := SyncTransactionChainFrom(peer, t.TokenID, w, log); syncErr != nil {
					return fmt.Errorf("failed to sync token chain for %s: %w", t.TokenID, syncErr), false
				}
				continue
			}

			if tokenDetails.TransactionID != t.PreviousTransactionID {
				log.Debug("transaction ID mismatch, syncing chain",
					"tokenID", t.TokenID,
					"existingTransactionID", tokenDetails.TransactionID,
					"incomingTransactionID", t.PreviousTransactionID,
				)
			}
			if tokenDetails.TransactionID != t.PreviousTransactionID {
				return fmt.Errorf("transaction ID mismatch for token %s: %s != %s", t.TokenID, tokenDetails.TransactionID, t.PreviousTransactionID), false
			}
			previousTransaction, err := w.GetTransactionByID(t.PreviousTransactionID)
			if err != nil {
				return fmt.Errorf("failed to get previous transaction for token %s: %w", t.TokenID, err), false
			}
			if tokenDetails.TransactionID != t.PreviousTransactionID {
				return fmt.Errorf("transaction ID mismatch for token %s: %s != %s", t.TokenID, tokenDetails.TransactionID, t.PreviousTransactionID), false
			}

			var previousTransactionInfo models.TransactionInfo
			if err := json.Unmarshal(previousTransaction.Info, &previousTransactionInfo); err != nil {
				return fmt.Errorf("failed to unmarshal previous transaction info for token %s: %w", t.TokenID, err), false
			}
			role := FindTokenRoleInTxn(t.TokenID, &previousTransactionInfo)
			if role == int16(models.GetTokenRoleID(constants.TokenRole_Pledge)) ||
				role == int16(models.GetTokenRoleID(constants.TokenRole_Burn)) ||
				role == int16(models.GetTokenRoleID(constants.TokenRole_Commit)) {
				return nil, false
			}
		}

		//add the same check for pledged tokens also, For pledge tokens fullnode will sync it from the quorum
		if isFullnode {
			for _, quorum := range txnInfo.Quorums {
				for _, t := range quorum.Tokens {
					tokenDetails, err := w.GetFullNodeRBTToken(t.TokenID)
					if err != nil {
						log.Debug("token not found locally, syncing full chain", "tokenID", t.TokenID)

						if syncErr, _ := SyncTransactionChainFrom(peer, t.TokenID, w, log); syncErr != nil {
							return fmt.Errorf("failed to sync token chain for %s: %w", t.TokenID, syncErr), false
						}

					}
					tokenDetails, err = w.GetFullNodeRBTToken(t.TokenID)
					if err != nil {
						return fmt.Errorf("failed to get token details for %s: %w", t.TokenID, err), false
					}
					if tokenDetails.TransactionID != t.PreviousTransactionID {
						return fmt.Errorf("transaction ID mismatch for token %s: %s != %s", t.TokenID, tokenDetails.TransactionID, t.PreviousTransactionID), false
					}

				}
			}
		}
	}

	return nil, true
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
//   - peer: pre-resolved peer for the initiator (for chain syncing)
//   - testnet, mainnet, localnet: network mode flags
//   - checkPinned: function to check IPFS pin status (tokenID, prevTxnID) → error
func ValidateTransaction(
	tx *models.Transactions,
	isFullnode bool,
	w *wallet.Wallet,
	log logger.Logger,
	initiatorDC types.DIDCrypto,
	quorumDCs map[string]types.DIDCrypto,
	peer *ipfsport.Peer,
	testnet bool,
	mainnet bool,
	localnet bool,
	checkPinned func(tokenID, previousTxnID string) error,
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

	if err := ValidateTokenOwnershipByPrevTxn(&txnInfo, isFullnode, w); err != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", err)
	}

	if syncErr, ok := TokenChainIntigrityCheck(&txnInfo, peer, isFullnode, w, log); syncErr != nil {
		return false, fmt.Errorf("ValidateTransaction: %w", syncErr)
	} else if !ok {
		return false, fmt.Errorf("ValidateTransaction: token chain sync failed")
	}
	// 7. ValidateTokenIDRelatedChecks for each RBT token in Tokens and CommittedTokens and pledged tokens
	if txnInfo.Tokens.RBT != nil {
		for _, token := range txnInfo.Tokens.RBT {
			if err := ValidateTokenIDRelatedChecks(token.TokenID, isFullnode, w, testnet, mainnet, localnet, log); err != nil {
				return false, fmt.Errorf("ValidateTransaction: token %s: %w", token.TokenID, err)

			}
		}
	}

	for _, t := range txnInfo.CommittedTokens {
		if err := ValidateTokenIDRelatedChecks(t.TokenID, isFullnode, w, testnet, mainnet, localnet, log); err != nil {
			return false, fmt.Errorf("ValidateTransaction: committed token %s: %w", t.TokenID, err)
		}
	}

	//Both Fullnode as well as quorum will do the ValidateTokenIDRelatedChecks checks for pledged tokens

	for _, quorum := range txnInfo.Quorums {
		for _, t := range quorum.Tokens {
			if err := ValidateTokenIDRelatedChecks(t.TokenID, isFullnode, w, testnet, mainnet, localnet, log); err != nil {
				return false, fmt.Errorf("ValidateTransaction: quorum %s token %s: %w", quorum.Did, t.TokenID, err)

			}
		}
	}

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
