package core

import (
	"fmt"

)

func (c *Core) FiveBlocksPassCheck(token string, tokenType int, creditEarnBlockNum uint64) (bool, error) {
	var err error
	// Get the latest token block
	latestBlock := c.w.GetLatestTokenBlock(token, tokenType)
	if latestBlock == nil {
		return false, fmt.Errorf("failed to get latest token block")
	}

	// Get the block number of the latest block
	latestBlockNum, err := latestBlock.GetBlockNumber(token)
	if err != nil {
		c.log.Error("Failed to get block number", "err", err)
		return false, fmt.Errorf("failed to get block number: %w", err)
	}

	// Check if the condition is met
	if latestBlockNum >= creditEarnBlockNum+5 {
		return true, nil
	}
	return false, nil
}

