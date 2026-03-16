package core

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types/models"
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

func (c *Core) PreviousTransactionIDIntegrityCheck(transactionID string) error {
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
			if level < constants.FaucetRBT_Level_Offset {
				return fmt.Errorf(
					"invalid testnet token level %d: testnet level must be >= %d",
					level, constants.FaucetRBT_Level_Offset,
				)
			}
			mapLevel = level - constants.FaucetRBT_Level_Offset
		} else if c.localnet {
			network = "localnet"
			if level <= constants.LocalRBT_Level {
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
		c.log.Debug("token content validated for "+network, tokenContent)
	}

	MaxPossiblePartTokenNumber := parts.MaxPossiblePartsIndexByMaxDecimalPlaces(uint(MaxDecimalPlaces))
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

func (c *Core) IsparentTokenBurnt(isFullNode bool, tokenID string) (error, bool) {
	
	if isFullNode {
	//First try to get the parent token from the tokens table,If token details are not there in the FullNodeRBT table
    //then it should compute  the parent tokenID from the given tokenID(currently assume a place holder function for that), 
    //then from the tokenchain table we have to get the genesis txnID of the given tokenID(assume a place holder function for that), 
    //then from the transactions table we have to get transactionInfo corresponding to genesisTxnID, 
    // then check whether the parent tokenID is there in the committed tokens list. 
    // if it is ther it should output nil,true. If it is not there it should output nil,false. 
    // For any other error it should output error,false.
		var parentTokenID string

		tokenDetails, err := c.w.GetFullNodeRBTToken(tokenID)
		if err == nil {
			if !tokenDetails.ParentTokenID.Valid || tokenDetails.ParentTokenID.String == "" {
				//instead of return, it should compute parent tokenID from the tokenID
						// TODO: replace with proper parent tokenID computation once available
			partTokenID := parts.TokenID(tokenID)
			parentPtr := partTokenID.Parent()
			if parentPtr == nil {
				return nil, false
			}
			parentTokenID = parentPtr.String()
				
			}
			parentTokenID = tokenDetails.ParentTokenID.String
		} else {
			// TODO: replace with proper parent tokenID computation once available
			partTokenID := parts.TokenID(tokenID)
			parentPtr := partTokenID.Parent()
			if parentPtr == nil {
				return nil, false
			}
			parentTokenID = parentPtr.String()
		}

		// TODO: replace with proper fullnode genesis txnID retrieval once available
		genesisTx, _, err := c.w.GetFullNodeTransactionAndRoleAtHeight(tokenID, 0)
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
	return nil, false
}
