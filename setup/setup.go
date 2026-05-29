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
	APIStart                      string = "/api/start"
	APIShutdown                   string = "/api/shutdown"
	APIPing                       string = "/api/ping"
	APIAddBootStrap               string = "/api/add-bootstrap"
	APIRemoveBootStrap            string = "/api/remove-bootstrap"
	APIRemoveAllBootStrap         string = "/api/remove-all-bootstrap"
	APIGetAllBootStrap            string = "/api/get-all-bootstrap"
	APIGetAllTokens               string = "/api/getalltokens"
	APIAddQuorum                  string = "/api/addquorum"
	APIGetAllQuorum               string = "/api/getallquorum"
	APIRemoveAllQuorum            string = "/api/removeallquorum"
	APISetupQuorum                string = "/api/setup-quorum"
	APIGenerateLocalRBT           string = "/api/generate-local-rbt"
	APIGenerateMainnetRBT         string = "/api/generate-mainnet-rbt"
	APISetupDID                   string = "/api/setup-did"
	APIFetchSmartContract         string = "/api/fetch-smart-contract"
	APIPeerID                     string = "/api/get-peer-id"
	APICheckQuorumStatus          string = "/api/check-quorum-status"
	APIAddPeerDetails             string = "/api/add-peer-details"
	APIGenerateFaucetTestToken    string = "/api/generate-faucettest-token"
	APIAddPeerDetailsFromExplorer string = "/api/add-peer-details-from-explorer"

	// signatures endpoints
	APISignatureResponse string = "/rubix/v1/signature"
	APIArbitrarySign     string = "/rubix/v1/signature/arbitrary"
	APISignVerification  string = "/rubix/v1/signature/verify"

	// Endpoints on DID module
	APICreateDID   string = "/rubix/v1/dids/create"
	APIGetAllDID   string = "/rubix/v1/dids"
	APIRegisterDID string = "/rubix/v1/dids/{did}/register"

	// DID balances
	APIGetDIDBalance string = "/rubix/v1/dids/{did}/balances"
	APIGetRbtByDid   string = "/rubix/v1/dids/{did}/balances/rbt"
	APIGetFtByDid    string = "/rubix/v1/dids/{did}/balances/ft"
	APIGetNftByDid   string = "/rubix/v1/dids/{did}/balances/nft"

	APIRemoveStaleDID string = "/api/remove-stale-did"

	APIGenerateSmartContract string = "/rubix/v1/smart_contracts/generate"
	APIListSmartContracts    string = "/rubix/v1/smart_contracts"
	APIGetSmartContractChain string = "/rubix/v1/smart_contracts/{contract_id}/chain"
	APICreateNFT             string = "/rubix/v1/nfts/generate"
	APIListNFTs              string = "/rubix/v1/nfts"
	APIGetNFTChain           string = "/rubix/v1/nfts/{nft_id}/chain"
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
