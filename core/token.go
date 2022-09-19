package core

func (c *Core) getTokens(did string, amount float64) ([]string, []string, bool) {
	return nil, nil, true
}

func (c *Core) removeTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: remove the tokens from the bank
	return nil
}

func (c *Core) releaseTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: releae the tokens which is lokced for the transaction
	return nil
}
