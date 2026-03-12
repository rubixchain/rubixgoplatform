package constants

const BlockVersion int = 1
const MaxSupportedDecimalPlaces int = 3

const (
	NetworkID_RBT_Mainnet string = "premint"
	NetworkID_RBT_Testnet string = "testrbt"
	NetworkID_RBT_Local   string = "local"
)

const (
	FaucetRBT_Level_Offset int = 50000
	LocalRBT_Level         int = 10000
)

// DB types
const (
	DBType_PostgreSQL string = "postgresql"
	DBType_SQLite     string = "sqlite"
)

// Default PostgreSQL values
const (
	DefaultPostgresUsername string = "rubix"
	DefaultPostgresPassword string = "rubixpass"
	DefaultPostgresDBName   string = "rubix"
	DefaultPostgresPort     uint64 = 5433
	DefaultPostgresHost     string = "localhost"
)

// Network mode
const (
	NetworkMode_Mainnet string = "mainnet"
	NetworkMode_Testnet string = "testnet"
	NetworkMode_Local   string = "local"
)

// Supported crypto algorithms for DID (Address)
const DidAlgo_SECP256K1 = "secp256k1"

// List of Token Roles
const (
	TokenRole_Mint     = "mint"
	TokenRole_Transfer = "transfer"
	TokenRole_Execute  = "execute"
	TokenRole_Deploy   = "deploy"
	TokenRole_Burn     = "burn"
	TokenRole_Commit   = "commit" // TODO: need a better name for it
	TokenRole_Uncommit = "uncommit"
	TokenRole_Pledge   = "pledge"
	TokenRole_Unpledge = "unpledge"
)

// List of Token Types
const (
	TokenType_RBT           = "rbt"
	TokenType_NFT           = "nft"
	TokenType_FT            = "ft"
	TokenType_SmartContract = "smart_contract"
)

// Token Statuses
const (
	TokenStatus_Free             = 0
	TokenStatus_Locked           = 1
	TokenStatus_Burnt            = 2
	TokenStatus_Committed        = 3
	TokenStatus_Pledged          = 4
	TokenStatus_UnPledged        = 5
	TokenStatus_Transferred      = 6
	TokenStatus_Generated        = 7
	TokenStatus_Deployed         = 8
	TokenStatus_Fetched          = 9
	TokenStatus_Executed         = 10
	TokenStatus_Orphaned         = 11
	TokenStatus_ChainSyncIssue   = 12
	TokenStatus_PledgeIssue      = 13
	TokenStatus_BeingDoubleSpent = 14
	TokenStatus_PinnedAsService  = 15
	TokenStatus_BurntForFT       = 16
	TokenStatus_Pending          = 17
	TokenStatus_QuorumPledged    = 20
	TokenStatus_Seed             = 99
)

// IPFS provider detail function identifiers
const (
	ProviderFunc_Pin   = iota + 1 // 1
	ProviderFunc_UnPin            // 2
	ProviderFunc_Cat              // 3
	ProviderFunc_Get              // 4
	ProviderFunc_Add              // 5
)

// IPFS provider detail role identifiers
const (
	ProviderRole_Owner                  = iota + 1 // 1
	ProviderRole_Quorum                            // 2
	ProviderRole_PrevSender                        // 3
	ProviderRole_Receiver                          // 4
	ProviderRole_ParentTokenLock                   // 5
	ProviderRole_DID                               // 6
	ProviderRole_Staking                           // 7
	ProviderRole_Pledging                          // 8
	ProviderRole_QuorumPin                         // 9
	ProviderRole_QuorumUnpin                       // 10
	ProviderRole_ParentTokenPinByQuorum            // 11
	ProviderRole_Pinning                           // 12
	ProviderRole_FullNode                          // 13
)

// Token operation mode
const (
	TokenMode_Send = 0
	TokenMode_Recv = 1
)

// Token chain sync status
const (
	TokenSync_Unrequired = 0
	TokenSync_Incomplete = 1
	TokenSync_Completed  = 2
)

// Requests Status
const (
	RequestStatus_Initated   = 0
	RequestStatus_InProgress = 1
	RequestStatus_Completed  = 2
	RequestStatus_Unknown    = 3
)
