package main

import "fmt"

func main() {
	c, cfg := setupDev()
	defer c.StopCore()

	w := c.GetWallet()
	w.SetDidDir(cfg.DidDir)
	fmt.Println("Core + IPFS ready")

	dids := createDIDs(c, 3)

	allTokenIDs, didTokens := mintTokens(w, &cfg, dids[:2], 5)

	fmt.Println("\nVALIDATION START")
	validateMintedTokens(allTokenIDs)

	runTransferRounds(w, &cfg, dids, didTokens)

	validatePostTransfer(w, dids, didTokens)

	fmt.Println("\nALL CHECKS PASSED - 3-DID multi-hop transfer simulation complete")
	RunStressTests(w, &cfg, dids, didTokens)
}
