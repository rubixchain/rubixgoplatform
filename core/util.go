package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
)

const (
	RBTString           string = "rbt"
	NFTString           string = "nft"
	PartString          string = "part"
	DataString          string = "data"
	SmartContractString string = "sc"
	FTString            string = "ft"
)

func (c *Core) getTotalAmountFromTokenHashes(tokenHashes []string) (float64, error) {
	var totalAmount float64 = 0.0

	for _, tokenHash := range tokenHashes {
		walletToken, err := c.w.ReadToken(tokenHash)
		if err != nil {
			return 0.0, fmt.Errorf("getTotalAmountFromTokenHashes: failed to read token %v, err: %v", tokenHash, err)
		}

		totalAmount += floatPrecision(walletToken.TokenValue, MaxDecimalPlaces)
	}

	return floatPrecision(totalAmount, MaxDecimalPlaces), nil
} 

func (c *Core) RACPartTokenType() int {
	if c.testNet {
		return rac.RacTestPartTokenType
	}
	return rac.RacPartTokenType
}
func (c *Core) RACFTType() int {
	if c.testNet {
		return rac.RacTestFTType
	}
	return rac.RacFTType
}

func (c *Core) TokenType(tt string) int {
	switch tt {
	case RBTString:
		if c.testNet {
			return token.TestTokenType
		}
		return token.RBTTokenType
	case NFTString:
		if c.testNet {
			return token.TestNFTTokenType
		}
		return token.NFTTokenType
	case PartString:
		if c.testNet {
			return token.TestPartTokenType
		}
		return token.PartTokenType
	case DataString:
		if c.testNet {
			return token.TestDataTokenType
		}
		return token.DataTokenType
	case SmartContractString:
		return token.SmartContractTokenType
	case FTString:
		return token.FTTokenType
	}
	return token.RBTTokenType
}
