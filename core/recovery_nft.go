package core

import (
	"encoding/hex"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/util"
)

// recoveryChildSuffixLen is how many characters of the request
// transaction ID go into the child NFT ID (96 bits).
const recoveryChildSuffixLen = 24

// RecoveryNFTID returns the Recovery NFT ID for this node's network.
// On localnet it comes from config; an empty result means recovery is off.
func (c *Core) RecoveryNFTID() string {
	switch {
	case c.mainnet:
		return constants.RecoveryNFTID_Mainnet
	case c.testnet:
		return constants.RecoveryNFTID_Testnet
	case c.localnet:
		return c.cfg.LocalnetRecoveryNFTID
	}
	return ""
}

// RecoveryChildNFTID builds the child NFT ID for a recovery request.
// It keeps the first 22 characters of the Recovery NFT ID and adds the
// last 24 characters of the request transaction ID, so the result is
// still a 46-character CID that every node can work out on its own.
func RecoveryChildNFTID(recoveryNFTID, requestTxID string) (string, error) {
	if err := util.ValidateCIDFormat(recoveryNFTID); err != nil {
		return "", fmt.Errorf("invalid recovery NFT ID: %w", err)
	}
	if _, err := hex.DecodeString(requestTxID); err != nil || len(requestTxID) != 64 {
		return "", fmt.Errorf("invalid request transaction ID: %s", requestTxID)
	}
	keep := len(recoveryNFTID) - recoveryChildSuffixLen
	return recoveryNFTID[:keep] + requestTxID[len(requestTxID)-recoveryChildSuffixLen:], nil
}
