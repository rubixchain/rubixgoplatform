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
	DefaultPostgresPort     uint64 = 5500
	DefaultPostgresHost     string = "localhost"
)

// Network mode
const (
	NetworkMode_Mainnet  string = "mainnet"
	NetworkMode_Testnet  string = "testnet"
	NetworkMode_Localnet string = "localnet"
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

// Token Statuses -- grouped by lifecycle stage
const (
	// Active states
	TokenStatus_Free = iota
	TokenStatus_Locked

	// Lifecycle -- creation & acquisition
	TokenStatus_Generated
	TokenStatus_Fetched

	// Lifecycle -- transfer & commitment
	TokenStatus_Transferred
	TokenStatus_Committed

	// Pledge states
	TokenStatus_Pledged
	TokenStatus_QuorumPledged

	// Terminal / destruction
	TokenStatus_Burnt
	TokenStatus_BurntForFT

	// Smart contract lifecycle
	TokenStatus_Deployed
	TokenStatus_Executed
	TokenStatus_PinnedAsService

	// Error / edge states
	TokenStatus_Orphaned
	TokenStatus_ChainSyncIssue
	TokenStatus_BeingDoubleSpent

	// Special
	TokenStatus_Seed = 99
)

// Token sync status constants.
const (
	SyncStatus_Completed  = 0
	SyncStatus_Incomplete = 1
	SyncStatus_Unrequired = 2
)

// Transaction mode constants.
const (
	TxnMode_Send            = 0
	TxnMode_Recv            = 1
	TxnMode_FTTransfer      = 2
	TxnMode_RBTSelfTransfer = 3
	TxnMode_FTSelfTransfer  = 4
	TxnMode_PinningService  = 5
	TxnMode_Deploy          = 6
	TxnMode_Execute         = 7
)

// Storage table name constants.
// These map old wallet storage names to PostgreSQL table names.
const (
	Storage_Tokens               = "tokens"
	Storage_Transactions         = "transactions"
	Storage_FTTokens             = "tokens" // FT tokens live in the main tokens table (token_type distinguishes)
	Storage_FTTransactionHistory = "ft_transaction_history"
	Storage_DIDPeer              = "did_peer_map"
	Storage_DIDs                 = "dids"
	Storage_FTs                  = "fts" // FT definitions (name, count, creator)
	Storage_FTTokenFixResult     = "ft_token_fix_result"
)

// Requests Status
const (
	RequestStatus_Initated   = 0
	RequestStatus_InProgress = 1
	RequestStatus_Completed  = 2
	RequestStatus_Unknown    = 3
)

const (
	DidPrefix = "bafybmi"
	DidLength = 59
)
