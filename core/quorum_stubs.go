package core

import "github.com/rubixchain/rubixgoplatform/block"

// checkIsPledged returns true if the block indicates the token is currently pledged.
// TODO(phase07): replace with DB token status check.
func (c *Core) checkIsPledged(blk *block.Block) bool {
	return false
}

