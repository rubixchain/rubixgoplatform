package core

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
