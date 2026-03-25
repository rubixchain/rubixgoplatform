package command

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// getNetworkMode returns the network mode string from CLI flags.
// Exactly one of testnet, mainnet, localnet must be true.
func getNetworkMode(testnet, mainnet, localnet bool) (string, error) {
	count := 0
	if testnet {
		count++
	}
	if mainnet {
		count++
	}
	if localnet {
		count++
	}
	if count != 1 {
		return "", fmt.Errorf("exactly one network mode must be selected (--testnet, --mainnet, or --localnet)")
	}
	switch {
	case testnet:
		return constants.NetworkMode_Testnet, nil
	case mainnet:
		return constants.NetworkMode_Mainnet, nil
	default:
		return constants.NetworkMode_Localnet, nil
	}
}

// dumpTokenChain dumps the token chain for a given token.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) dumpTokenChain() {
	cmd.log.Error("dumpTokenChain: not implemented in PostgreSQL build")
}

// decodeTokenChain decodes and displays the token chain for a given token.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) decodeTokenChain() {
	cmd.log.Error("decodeTokenChain: not implemented in PostgreSQL build")
}

// dumpSmartContractTokenChain dumps the smart contract token chain.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) dumpSmartContractTokenChain() {
	cmd.log.Error("dumpSmartContractTokenChain: not implemented in PostgreSQL build")
}

// getTokenBlock retrieves a specific token block.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) getTokenBlock() {
	cmd.log.Error("getTokenBlock: not implemented in PostgreSQL build")
}

// getSmartContractData retrieves smart contract data from the latest block.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) getSmartContractData() {
	cmd.log.Error("getSmartContractData: not implemented in PostgreSQL build")
}

// releaseAllLockedTokens releases all tokens currently locked on the node.
// TODO(phase11-upstream): implement via wallet lock release.
func (cmd *Command) releaseAllLockedTokens() {
	cmd.log.Error("releaseAllLockedTokens: not implemented in PostgreSQL build")
}

// dumpFTTokenchain dumps the FT token chain.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) dumpFTTokenchain() {
	cmd.log.Error("dumpFTTokenchain: not implemented in PostgreSQL build")
}

// dumpNFTTokenChain dumps the NFT token chain.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (cmd *Command) dumpNFTTokenChain() {
	cmd.log.Error("dumpNFTTokenChain: not implemented in PostgreSQL build")
}
