package core

import (
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
)

const (
	RBTString           string = "rbt"
	NFTString           string = "nft"
	PartString          string = "part"
	DataString          string = "data"
	SmartContractString string = "sc"
)

func (c *Core) RACPartTokenType() int {
	if c.testNet {
		return rac.RacTestPartTokenType
	}
	return rac.RacPartTokenType
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
	}
	return token.RBTTokenType
}
