package main

import (
	"fmt"
	"os"

	// Try these import paths depending on your build environment:
	// "rubixgoplatform/block"
	// "rubixgoplatform/core/wallet"
	"block"
	"core/wallet"
)

func main() {
	w, err := wallet.NewWallet("/home/rubix/Rubix/Node/creator/api_config.json")
	if err != nil {
		fmt.Println("Failed to initialize wallet:", err)
		os.Exit(1)
	}

	token := "TRI"
	tokenType := 10 // FTTokenType

	blocks, _, err := w.GetAllTokenBlocks(token, tokenType, "")
	if err != nil {
		fmt.Println("Error fetching token blocks:", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d blocks for token %s:\n", len(blocks), token)
	for i, blk := range blocks {
		b := block.InitBlock(blk, nil)
		id, _ := b.GetBlockID(token)
		fmt.Printf("Block %d: ID=%s\n", i, id)
	}
}
