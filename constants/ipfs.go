package constants

const (
	RubixServerPort  uint16 = 20000
	SendPort         uint16 = 21000
	RecvPort         uint16 = 22000
	IPFSPort         uint16 = 5002
	SwarmPort        uint16 = 4002
	IPFSAPIPort      uint16 = 8081
	MaxPeerConn      uint16 = 1000
	PostgresBasePort uint16 = 5433
)

var (
	MainnetBootstrapNodes []string = []string{
		"/ip4/161.35.169.251/tcp/4001/p2p/12D3KooWPhZEYEw4jG3kSRuwgMEHcVt7KMkm1ui2ddu4fgSgwvDq",
		"/ip4/103.127.158.120/tcp/4001/p2p/12D3KooWSQ94HRDzFf6W2rp7P8gzP6efZQHTaSU8uaQjskVBHiWP",
		"/ip4/172.104.191.191/tcp/4001/p2p/12D3KooWFudnWZY1v1m4YXCzDWZSbNt7nvf5F42uzM6vErZ4NwqJ",
	}

	TestnetBootstrapNodes []string = []string{
		"/ip4/103.209.145.177/tcp/4001/p2p/12D3KooWD8Rw7Fwo4n7QdXTCjbh6fua8dTqjXBvorNz3bu7d9xMc",
		"/ip4/98.70.52.158/tcp/4001/p2p/12D3KooWQyWFABF3CKFnzX85hf5ZwrT5zPsy4rWHdGPZ8bBpRVCK",
		"/ip4/20.244.16.143/tcp/4001/p2p/12D3KooWAydFDJeSW5qupmp3AjRxc82Dq1AnjfJT1zwy4hg2TuNn",
		"/ip4/40.81.232.217/tcp/4001/p2p/12D3KooWK6V21GQotbub3cfgb5qAK1uUoUGPexf3vsLqw6yBJfen",
	}
)

// Token Provider Roles
const (
	TokenProviderFunc_Add = 1
	TokenProviderFunc_Pin = 2
)

// Token pinning role constants (stored in ipfs_providers.role column).
const (
	TokenProviderRole_Owner           = 1
	TokenProviderRole_Quorum          = 2
	TokenProviderRole_Receiver        = 3
	TokenProviderRole_PrevSender      = 4
	TokenProviderRole_ParentTokenLock = 5
	TokenProviderRole_ParentTokenPin  = 6
	TokenProviderRole_DID             = 7
	TokenProviderRole_Staking         = 8
	TokenProviderRole_Pledging        = 9
	TokenProviderRole_QuorumPin       = 10
	TokenProviderRole_QuorumUnpin     = 11
	TokenProviderRole_Pinning         = 12
	TokenProviderRole_FullNode        = 13
)

// IPFS provider resource type constants.
// These identify what kind of resource a CID represents.
const (
	IPFSResourceTokenState    = "token_state"
	IPFSResourceTokenChain    = "token_chain"
	IPFSResourceNFT           = "nft"
	IPFSResourceSmartContract = "smart_contract"
	IPFSResourceDID           = "did"
	IPFSResourceGeneric       = "generic"
	IPFSResourceFT            = "ft"
)

// IPFS provider operation constants.
const (
	IPFSProviderOpAdd   = "add"
	IPFSProviderOpPin   = "pin"
	IPFSProviderOpUnpin = "unpin"
)

// IPFS provider status constants.
const (
	IPFSProviderStatusActive   = "active"
	IPFSProviderStatusUnpinned = "unpinned"
)
