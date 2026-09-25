package setup

import "github.com/golang-jwt/jwt/v5"

const (
	DIDConfigField string = "did_config"
)

const (
	ChanllegeTokenType string = "challengeToken"
	AccessTokenType    string = "accessToken"
)

const (
	APIStart                   string = "/rubix/v1/node/start"
	APIShutdown                string = "/rubix/v1/node/shutdown"
	APIPing                    string = "/rubix/v1/node/ping"
	APIAddBootStrap            string = "/rubix/v1/bootstrap/add"
	APIRemoveBootStrap         string = "/rubix/v1/bootstrap/remove"
	APIRemoveAllBootStrap      string = "/rubix/v1/bootstrap/remove_all"
	APIGetAllBootStrap         string = "/rubix/v1/bootstrap"
	APIGetAllTokens            string = "/rubix/v1/tokens"
	APIAddQuorum               string = "/rubix/v1/quorums/add"
	APIGetAllQuorum            string = "/rubix/v1/quorums"
	APIRemoveAllQuorum         string = "/rubix/v1/quorums/remove_all"
	APISetupQuorum             string = "/rubix/v1/quorums/setup"
	APIGenerateLocalRBT        string = "/rubix/v1/tokens/generate_local_rbt"
	APISetupDID                string = "/rubix/v1/dids/setup"
	APIFetchSmartContract      string = "/rubix/v1/smart_contracts/fetch"
	APIPeerID                  string = "/rubix/v1/node/peer_id"
	APICheckQuorumStatus       string = "/rubix/v1/quorums/status"
	APIAddPeerDetails          string = "/rubix/v1/node/add_peers"
	APIGenerateFaucetTestToken string = "/rubix/v1/tokens/generate_faucet_test"

	// signatures endpoints
	APISignatureResponse string = "/rubix/v1/signature"
	APIArbitrarySign     string = "/rubix/v1/signature/arbitrary"
	APISignVerification  string = "/rubix/v1/signature/verify"

	// Endpoints on DID module
	APICreateDID   string = "/rubix/v1/dids/create"
	APIGetAllDID   string = "/rubix/v1/dids"
	APIRegisterDID string = "/rubix/v1/dids/{did}/register"
	// APIGetPubKeyByDID is the reverse of creating a DID from a public key
	// (APICreateDID with a `public_key` body): given the DID it returns the
	// public key the DID was derived from.
	APIGetPubKeyByDID string = "/rubix/v1/dids/{did}/public_key"

	// DID balances
	APIGetDIDBalance string = "/rubix/v1/dids/{did}/balances"
	APIGetRbtByDid   string = "/rubix/v1/dids/{did}/balances/rbt"
	APIGetFtByDid    string = "/rubix/v1/dids/{did}/balances/ft"
	APIGetNftByDid   string = "/rubix/v1/dids/{did}/balances/nft"

	APIGenerateSmartContract string = "/rubix/v1/smart_contracts/generate"
	APIListSmartContracts    string = "/rubix/v1/smart_contracts"
	APIGetSmartContractChain string = "/rubix/v1/smart_contracts/{contract_id}/chain"
	APICreateNFT             string = "/rubix/v1/nfts/generate"
	APIListNFTs              string = "/rubix/v1/nfts"
	APIGetNFTChain           string = "/rubix/v1/nfts/{nft_id}/chain"
	APIGetNFTProperties      string = "/rubix/v1/nfts/{nft_id}/properties"
	APIGetChildNFTs          string = "/rubix/v1/nfts/{nft_id}/children"
	APIGetParentNFT          string = "/rubix/v1/nfts/{nft_id}/parent"
	APISubscribecontract     string = "/rubix/v1/smart_contracts/subscribe"
	APISubscribeNFT          string = "/rubix/v1/nfts/subscribe"
	APIFetchNft              string = "/rubix/v1/fetch-nft"
	APIRegisterCallBackURL   string = "/rubix/v1/smart_contracts/register_callback"

	// Transactions endpoints
	APITransaction          string = "/rubix/v1/tx"
	APIGetTransactionByID   string = "/rubix/v1/tx/{tx_id}"
	APISyncTransactionChain string = "/rubix/v1/sync-transaction-chain"
	APIGetTransactionsByDID string = "/rubix/v1/tx/{did}/{token_type}"

	// FT endpoints
	APIListFT   string = "/rubix/v1/fts"
	APICreateFT string = "/rubix/v1/fts/mint"

	// Fullnode endpoints
	APISyncTransactionInfoFromFullnode string = "/rubix/v1/fullnode/sync"
	APIRecoverFromFullnode             string = "/rubix/v1/fullnode/recover"
	// APIRecoverChallenge issues a one-time, single-use nonce the recovering
	// node signs to prove ownership of the DID before any chain data is served.
	APIRecoverChallenge string = "/rubix/v1/fullnode/recover-challenge"

	// Operator-facing HTTP endpoint on the recovering normal node that triggers
	// fullnode-backed recovery (orchestrates fetching fullnodes.json, dialing
	// a fullnode over libp2p, calling APIRecoverFromFullnode in a paginated
	// loop, and persisting results locally).
	APIRecoverWalletFromFullnode string = "/rubix/v1/sync"
)

// jwt.RegisteredClaims

type BearerToken struct {
	TokenType string `json:"type"`
	DID       string `json:"did"`
	PeerID    string `json:"peerId"`
	Random    string `json:"random"`
	Root      bool   `json:"root"`
	jwt.RegisteredClaims
}
