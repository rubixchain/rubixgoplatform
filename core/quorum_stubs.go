package core

// checkIsPledged returns true if the block stub indicates the token is currently pledged.
// TODO(phase07): replace with DB token status check (constants.TokenStatus_Pledged).
// The block package has been removed; this stub accepts BlockStub from wallet package.
func (c *Core) checkIsPledged(blk interface{}) bool {
	return false
}
