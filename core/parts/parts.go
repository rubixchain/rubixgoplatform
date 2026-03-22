package parts

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// CollectRBTTokens is a PURE denomination-selection function:
//   - No DB reads (ownedRBTTokens and denomMap are pre-fetched and passed in by the caller)
//   - No DB writes (no storeGenesisTx, no LockTokensByID, no CreateRBTToken)
//   - No pubsub publish (util.PublishTransaction removed)
//   - IPFS Add/Pin calls remain in the split helpers (IPFS upload before DB persistence)
//
// Callers must:
//  1. Pre-fetch tokens via w.LockTokensForSplit
//  2. Pre-fetch denomMap via w.GetTokenDenomArray
//  3. Persist returned childRecords via w.PersistGenesisBatch
//  4. Burn parentsToBurn tokens in the DB
func CollectRBTTokens(
	dc types.DIDCrypto,
	w *wallet.Wallet,
	transferAmount float64,
	ownedRBTTokens []models.Token,
	denomMap map[types.DenomValue]types.DenomCount,
	network string,
	log logger.Logger,
) (
	tokensForTransfer []*models.TokenInfo,
	childRecords []wallet.GenesisMintRecord,
	parentsToBurn []string,
	remainingDenomMap map[types.DenomValue]types.DenomCount,
	err error,
) {
	var splitOps []SplitOp = make([]SplitOp, 0)
	tokensForTransfer = make([]*models.TokenInfo, 0)
	childRecords = make([]wallet.GenesisMintRecord, 0)
	parentsToBurn = make([]string, 0)

	var did string = dc.GetDID()

	// Check if transfer amount doesn't exceed maximum supported decimal places.
	decimalPlaces := strconv.FormatFloat(transferAmount, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > constants.MaxSupportedDecimalPlaces {
		return nil, nil, nil, nil, fmt.Errorf("transaction amount exceeds %v decimal places", constants.MaxSupportedDecimalPlaces)
	}

	// In-memory balance check from the caller-provided denomMap (no DB read).
	var totalBalance float64
	for denom, count := range denomMap {
		totalBalance += float64(denom) * float64(count)
	}
	if totalBalance < transferAmount {
		return nil, nil, nil, nil, fmt.Errorf(
			"CollectRBTTokens: insufficient balance: current balance %v, transfer amount %v",
			totalBalance, transferAmount,
		)
	}

	// Attempt to collect tokens which wouldn't require any splitting
	// by looking at the caller-provided denom map.
	var nonSplitTokenTransfer []models.Token = make([]models.Token, 0)

	nonSplitDenomArr, remainingBalanceDenomArr, remainingAmount, err := GetSplitAndNonsplitTokenDenom(denomMap, transferAmount)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: error occured while looking to fetch non-split token denom array for transfer, err: %v", err)
	}
	remainingDenomMap = remainingBalanceDenomArr

	if len(nonSplitDenomArr) != 0 {
		// In-memory filter: select tokens from ownedRBTTokens whose TokenValue matches
		// entries in nonSplitDenomArr, consuming counts as tokens are matched.
		denomCounters := make(map[types.DenomValue]types.DenomCount, len(nonSplitDenomArr))
		for dv, dc := range nonSplitDenomArr {
			denomCounters[dv] = dc
		}
		for _, tok := range ownedRBTTokens {
			needed, ok := denomCounters[tok.TokenValue]
			if !ok || needed <= 0 {
				continue
			}
			nonSplitTokenTransfer = append(nonSplitTokenTransfer, tok)
			denomCounters[tok.TokenValue]--
		}

		for _, nonSplit := range nonSplitTokenTransfer {
			tokensForTransfer = append(tokensForTransfer, &models.TokenInfo{
				TokenID:               nonSplit.TokenID,
				PreviousTransactionID: nonSplit.TransactionID,
			})
		}
	}

	var tokensToKeepList []models.Token = make([]models.Token, 0)
	var tokensBeingBurntList []models.Token = make([]models.Token, 0)

	if remainingAmount > rubixmath.ZeroFloat() {
		// For the remaining amount, proceed to build the denom tree and split accordingly.
		var remainingAvailableTokens []models.Token = removeTokensFromList(ownedRBTTokens, nonSplitTokenTransfer)

		// Build the tree.
		tokenDenomTree, err := BuildDenomTree(remainingAvailableTokens, did)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to get the denom tree for did: %v, err: %v", did, err)
		}

		for _, token := range tokenDenomTree.Leaves {
			if remainingAmount <= rubixmath.ZeroFloat() {
				break
			}

			heirarchicalID := token.HierarchicalID
			tokenValue := token.Token.TokenValue

			if tokenValue > remainingAmount {
				splitOp, err := planTokenSplit(heirarchicalID, remainingAmount, log)
				if err != nil {
					return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to plan the token split, err: %v", err)
				}

				splitOps = append(splitOps, splitOp...)
				remainingAmount = rubixmath.ZeroFloat()
			}
		}

		if remainingAmount > rubixmath.ZeroFloat() {
			return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: could not satisfy transfer amount, remaining: %v", remainingAmount)
		}

		tokenCache := make(map[string]models.Token)

		// Execute split operations. performTokenSplit now returns childMintRecords
		// (partial — TransactionID not set yet) and no longer persists to DB.
		for _, splitOp := range splitOps {
			partTokensToTransfer, tokensToKeep, tokensBeingBurnt, splitMintRecords, err := performTokenSplit(
				w, dc, splitOp, tokenCache, remainingBalanceDenomArr, network,
			)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: could not perform split at Level: %v, err: %v", splitOp.HierarchicalTokenID.Level(), err)
			}

			tokensToKeepList = append(tokensToKeepList, tokensToKeep...)
			tokensBeingBurntList = append(tokensBeingBurntList, tokensBeingBurnt...)
			childRecords = append(childRecords, splitMintRecords...)

			for _, partToken := range partTokensToTransfer {
				tokensForTransfer = append(tokensForTransfer, &models.TokenInfo{
					TokenID:               partToken.TokenID,
					PreviousTransactionID: partToken.TransactionID,
				})
			}
		}
	}

	// KeepList and BurntList are compared to find any common elements.
	// If found, the element is removed from KeepList since it is going to be burned.
	commonTokens := util.FindCommonElementsInList(tokensToKeepList, tokensBeingBurntList)
	tokensToKeepList = util.RemoveElementsFromList(tokensToKeepList, commonTokens)

	// Collect parent token IDs that need to be burned.
	for _, burnt := range tokensBeingBurntList {
		parentsToBurn = append(parentsToBurn, burnt.TokenID)
	}

	// Build genesis transaction info and signature for the split operation.
	// NOTE: storeGenesisTx and util.PublishTransaction are NOT called here.
	// The caller persists via w.PersistGenesisBatch.
	transactionInfo, signature, err := createGenesisTransaction(dc, tokensToKeepList, tokensBeingBurntList, did, network)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to get genesis transaction info, err: %v", err)
	}

	txID, err := wallet.ComputeTransactionID(transactionInfo)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to compute transaction id, err: %v", err)
	}

	infoBytes, err := models.SerializeTransactionInfo(transactionInfo)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to serialize transaction info, err: %v", err)
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to marshal signature, err: %v", err)
	}

	txRecord := &models.Transactions{
		ID:        txID,
		Info:      infoBytes,
		Signature: signatureBytes,
	}

	// Wire txRecord and TransactionID into each child GenesisMintRecord.
	for i := range childRecords {
		childRecords[i].TxRecord = txRecord
		childRecords[i].TokenChain.TransactionID = txID
		childRecords[i].Token.TransactionID = txID
	}

	return tokensForTransfer, childRecords, parentsToBurn, remainingDenomMap, nil
}

//This function returns the max possible parts index by the max decimal places, For 3 decimal places, it will be 1332
func MaxPossiblePartsIndexByMaxDecimalPlaces(maxDecimalPlaces uint) int {
	totalPossibleLevels := 2 * maxDecimalPlaces //Actually 2*maxDecimalPlaces+1 will be the total possible levels but 1st level is about whole token so we are ignoring
	sum := 0
	nodesInLevel := make([]int, totalPossibleLevels+1)
	nodesInLevel[0] = 1 // level 0 has 1 node
	for i := uint(1); i <= totalPossibleLevels; i++ {
		if i%2 == 1 {
			nodesInLevel[i] = 2 * nodesInLevel[i-1]
		} else {
			nodesInLevel[i] = 5 * nodesInLevel[i-1]
		}
		sum += nodesInLevel[i]

	}
	return sum

}
