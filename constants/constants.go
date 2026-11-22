package constants

const BlockVersion int = 1
const MaxSupportedDecimalPlaces int = 3

const (
	NetworkID_RBT_Mainnet string = "premint"
	NetworkID_RBT_Testnet string = "testrbt"
	NetworkID_RBT_Local   string = "local"
)

const (
	TestnetRBT_Level_Offset int = 50000
	LocalRBT_Level_Offset   int = 10000
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

// Recovery NFT IDs for mainnet and testnet. Both are placeholders for now.
// Update them by hand once the real Recovery NFTs are deployed.
const (
	RecoveryNFTID_Mainnet string = "QmbKP6dLYpD8LH9bjjztQGFMhpQULBZYZkCxVSwTqpAhPm"
	RecoveryNFTID_Testnet string = "QmbKP6dLYpD8LH9bjjztQGFMhpQULBZYZkCxVSwTqpAhPm"
)

// Recovery session states. States only move forward, so a higher value is a
// later state.
const (
	// The request was received and passed its checks.
	RecoveryState_Requested = iota + 1
	// The user deployed the ephemeral NFT, so commitments can be published.
	RecoveryState_CommitOpen
	// The commit window closed; quorums send chains.
	RecoveryState_SyncOpen
	// The sync window closed.
	RecoveryState_SyncClosed
	// Someone other than the user deployed the ephemeral NFT. This is final.
	RecoveryState_Blocked
)

// Recovery request rules.
const (
	RecoveryRequest_Purpose = "account recovery"
	// How old a request transaction may be, in seconds, when it reaches a node (Q6).
	RecoveryRequest_MaxAgeSeconds = 2 * 60
	// Minimum time, in seconds, between two requests from the same DID (Q10).
	RecoveryRequest_CooldownSeconds = 48 * 60 * 60
)

// Recovery windows (Q6), in seconds, counted from the request transaction
// epoch. They are the same on every node and network. The sync window starts
// when the commit window ends. A node acts on a deadline only after the grace
// period, to allow for clock drift.
const (
	RecoveryWindow_CommitSeconds = 10 * 60
	RecoveryWindow_SyncSeconds   = 30 * 60
	RecoveryWindow_GraceSeconds  = 60
	// Commitments are published at a random time in the first part of the
	// commit window, so that publishers do not execute the ephemeral NFT on
	// the same tip at once.
	RecoveryWindow_PublishSpreadSeconds = 5 * 60
)

// Recovery message types, sent on the ephemeral NFT.
const (
	RecoveryMessageType_Commitment = "commitment"
	RecoveryMessageType_Unpledged  = "unpledged"
	RecoveryMessageType_Challenge  = "challenge"
	RecoveryMessageType_Mismatch   = "mismatch"
	RecoveryMessageType_Completion = "completion"
)

// Recovery commitment Merkle tree. The leaf and node prefixes separate the two
// hash domains, as in RFC 6962 (Certificate Transparency), so a token leaf and
// an internal node can never be reinterpreted as one another.
const (
	RecoveryCommit_PrefixLeaf byte = 0x00
	RecoveryCommit_PrefixNode byte = 0x01
	// Length in bytes of every hash and root (SHA3-256).
	RecoveryCommit_HashSize = 32
)

// Recovery commitment committer types: which token set a commitment covers.
const (
	RecoveryCommitterType_Quorum   = "quorum"
	RecoveryCommitterType_Fullnode = "fullnode"
)

// Roles a node can have in a recovery session.
const (
	RecoveryRole_User     = "user"
	RecoveryRole_Quorum   = "quorum"
	RecoveryRole_Fullnode = "fullnode"
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

// MinTransactionEpochUnix is the Unix time in seconds for 2026-04-01 00:00:00 UTC.
// Valid transaction Epoch values must be greater than or equal to this (and not in the future).
const MinTransactionEpochUnix int = 1775001600
