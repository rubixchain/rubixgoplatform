package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/token"
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

func (c *Core) IsparentTokenBurnt(isFullNode bool) (error, bool) {
	//First try to get the parent token from the tokens table, if fullnode calls this function it should it from
	//the fullnode's tokens table.
	if isFullNode {

		return nil, false
	}
	return nil, true
}
