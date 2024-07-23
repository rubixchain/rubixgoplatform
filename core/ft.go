package core

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
)

func (c *Core) CreateFTs(dc did.DIDCrypto, tokenName string, numTokens int, numWholeTokens int, did string) ([]wallet.Token, error) {
	if dc == nil {
		return nil, fmt.Errorf("DID crypto is not initialized")
	}

	// Validate input parameters
	if numTokens <= 0 {
		return nil, fmt.Errorf("number of tokens to create must be greater than zero")
	}
	if numWholeTokens <= 0 {
		return nil, fmt.Errorf("number of whole tokens must be greater than zero")
	}

	// Fetch whole tokens using GetToken
	wholeTokens := make([]wallet.Token, 0, numWholeTokens)
	for i := 0; i < numWholeTokens; i++ {
		tkn, err := c.w.GetToken(fmt.Sprintf("%s-%d", tokenName, i), wallet.TokenIsFree) // Assuming token IDs are formatted this way
		if err != nil || tkn == nil {
			return nil, fmt.Errorf("failed to fetch whole token %d: %v", i, err)
		}
		wholeTokens = append(wholeTokens, *tkn)
	}

	// Calculate the value of each fractional token
	fractionalValue := float64(len(wholeTokens)) / float64(numTokens)

	// Create a slice to hold the new tokens
	newTokens := make([]wallet.Token, 0, numTokens)

	// Create RAC type for fractional tokens
	for i := 0; i < numTokens; i++ {
		// Use the TokenID of the whole token as the Parent
		parentTokenID := wholeTokens[i%len(wholeTokens)].TokenID // Use modulo to cycle through available whole tokens

		racType := &rac.RacType{
			Type:        rac.RacPartTokenType,
			DID:         did,
			TokenNumber: uint64(i),
			TotalSupply: 1, // Each fractional token is created individually
			TimeStamp:   time.Now().String(),
			PartInfo: &rac.RacPartInfo{
				Parent:  parentTokenID, // Use the fetched token ID
				PartNum: i,
				Value:   fractionalValue,
			},
		}

		// Create the RAC block
		racBlocks, err := rac.CreateRac(racType)
		if err != nil {
			c.log.Error("Failed to create RAC block", "err", err)
			return nil, err
		}

		// Expect one block to be created
		if len(racBlocks) != 1 {
			return nil, fmt.Errorf("failed to create RAC block")
		}

		// Update the signature of the RAC block
		err = racBlocks[0].UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update DID signature", "err", err)
			return nil, err
		}

		// Add the RAC block to the wallet
		racBlockData := racBlocks[0].GetBlock()
		fr := bytes.NewBuffer(racBlockData)
		pt, err := c.w.Add(fr, did, wallet.AddFunc)
		if err != nil {
			c.log.Error("Failed to add RAC token to IPFS", "err", err)
			return nil, err
		}

		// Create the new token
		token := &wallet.Token{
			TokenID:       pt,
			ParentTokenID: parentTokenID, // Link to the parent token
			TokenValue:    fractionalValue,
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}

		// Create the token in the wallet
		err = c.w.CreateToken(token)
		if err != nil {
			c.log.Error("Failed to create fractional token", "err", err)
			return nil, err
		}

		// Append the newly created token to the slice
		newTokens = append(newTokens, *token)
	}

	return newTokens, nil
}
