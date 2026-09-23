package core

import (
	"path"

	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/core/swarmkey"
)

// ipfsSwarmKeyFilename is the name initIPFS copies the swarm key to inside the
// IPFS repo, whichever network the key is for.
const ipfsSwarmKeyFilename string = "swarm.key"

// resolveNetworkIdentity works out which private network the node joined, from
// the swarm key the IPFS daemon loads. A key Rubix does not operate means a
// custom network, and on testnet such a node validates minters against
// CustomNetAllowedMinters rather than the built in list.
//
// Mainnet keeps the built in list whatever key it runs with, so a custom
// network can never loosen it.
//
// Called once at startup, after initIPFS has placed the key.
func (c *Core) resolveNetworkIdentity(ipfsDir string) {
	fingerprint, err := swarmkey.FingerprintFile(path.Join(ipfsDir, ipfsSwarmKeyFilename))
	if err != nil {
		// Leave customNetwork false so validation stays on the built in list.
		c.log.Error("Failed to read swarm key, keeping the built in minter allowlist", "err", err)
		return
	}

	if swarmkey.IsCanonical(c.networkMode, fingerprint) || !c.testnet {
		c.log.Info("Minter allowlist: built in list", "network", c.networkMode)
		return
	}

	c.customNetwork = true
	if len(minterallowlist.CustomNetAllowedMinters) == 0 {
		c.log.Warn("Custom testnet has no minter allowlist, every token will be processed whoever minted it")
		return
	}
	c.log.Info("Minter allowlist: custom testnet", "minters", len(minterallowlist.CustomNetAllowedMinters))
}
