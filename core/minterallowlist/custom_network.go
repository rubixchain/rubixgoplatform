package minterallowlist

import "sync/atomic"

// customNetwork records whether this node joined a private network Rubix does
// not operate, which it knows from the fingerprint of its IPFS swarm key.
//
// It is set once during startup, before the node serves any request, and read
// on every transaction validation thereafter. It lives here rather than being
// passed down because it selects between the tables in this package, and only
// the two allowlist decisions ever consult it: which minter table applies, and
// which token level offset the ids carry.
var customNetwork atomic.Bool

// SetCustomNetwork records the network the node joined. Called once at startup
// by the core, from Core.resolveNetworkIdentity.
func SetCustomNetwork(custom bool) {
	customNetwork.Store(custom)
}

// IsCustomNetwork reports whether the node is on a network Rubix does not
// operate. False on mainnet, on the Rubix testnet and on localnet, so those
// networks keep the built in tables and offsets.
func IsCustomNetwork() bool {
	return customNetwork.Load()
}
