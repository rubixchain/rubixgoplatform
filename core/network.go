package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/constants"
)

func (c *Core) ValidateTokenNetworkID(blk *block.Block, tokenID string) error {
	blockNumber, err := blk.GetBlockNumber(tokenID)
	if err != nil {
		return fmt.Errorf("ValidateTokenNetworkID: failed to get block number for token %s, err: %v", tokenID, err)
	}

	if blockNumber == 0 {
		networkID, err := blk.GetGenesisNetworkType(tokenID)
		if err != nil {
			return fmt.Errorf("ValidateTokenNetworkID: failed to get genesis network type for token %s, err: %v", tokenID, err)
		}

		if c.testnet {
			switch networkID {
			case constants.NetworkID_RBT_Testnet, constants.NetworkID_RBT_Local:
				// valid testnet network IDs
			default:
				return fmt.Errorf("ValidateTokenNetworkID:token network ID unsupported for testnet, found id: %s", networkID)
			}
		} else {
			switch networkID {
			case constants.NetworkID_RBT_Mainnet:
				// valid mainnet network ID
			default:
				return fmt.Errorf("ValidateTokenNetworkID: token network ID unsupported for mainnet, found id: %s", networkID)
			}
		}
	} else {
		return fmt.Errorf("ValidateTokenNetworkID: unexpected error: expected genesis block for token %s, found block number %d", tokenID, blockNumber)
	}

	return nil
}
