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

	// Mint extra tokens for the stress+fuzz harness only — not part of the main simulation.
	// Budget: S1(1) + S2(5) + S3(1) + fuzz-adversarial(1, shared) + fuzz-valid(10) = 18 min.
	// Original 10 tokens cover S1+S2+S3 (7 used). We need 11 more in availableTokens.
	// Minting 10 per DID (20 extra) gives: 30 total − 7 used = 23 available for fuzz.
	fmt.Println("\nMinting stress-harness tokens...")
	_, stressTokenMap := mintTokens(w, &cfg, dids[:2], 10)
	for _, d := range dids[:2] {
		didTokens[d] = append(didTokens[d], stressTokenMap[d]...)
	}

	RunStressTests(w, &cfg, dids, didTokens)
}
