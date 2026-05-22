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
