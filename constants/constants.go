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

// Token Statuses
const (
	TokenStatus_Free      = 0
	TokenStatus_Locked    = 1
	TokenStatus_Burnt     = 2
	TokenStatus_Committed = 3
	TokenStatus_Seed      = 99
)

// Requests Status
const (
	RequestStatus_Initated   = 0
	RequestStatus_InProgress = 1
	RequestStatus_Completed  = 2
	RequestStatus_Unknown    = 3
)
