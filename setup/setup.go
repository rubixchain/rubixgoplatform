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
	APIStart                            string = "/api/start"
	APIShutdown                         string = "/api/shutdown"
	APINodeStatus                       string = "/api/node-status"
	APIPing                             string = "/api/ping"
	APIAddBootStrap                     string = "/api/add-bootstrap"
	APIRemoveBootStrap                  string = "/api/remove-bootstrap"
	APIRemoveAllBootStrap               string = "/api/remove-all-bootstrap"
	APIGetAllBootStrap                  string = "/api/get-all-bootstrap"
	APIGetDIDChallenge                  string = "/api/getdidchallenge"
	APIGetDIDAccess                     string = "/api/logindid"
	APIGetAllTokens                     string = "/api/getalltokens"
	APIAddQuorum                        string = "/api/addquorum"
	APIGetAllQuorum                     string = "/api/getallquorum"
	APIRemoveAllQuorum                  string = "/api/removeallquorum"
	APISetupQuorum                      string = "/api/setup-quorum"
	APISetupService                     string = "/api/setup-service"
	APIGenerateLocalRBT                 string = "/api/generate-local-rbt"
	APIInitiateRBTTransfer              string = "/api/initiate-rbt-transfer"
	APIDumpTokenChainBlock              string = "/api/dump-token-chain"
	APISetupDID                         string = "/api/setup-did"
	APIMigrateNode                      string = "/api/migrate-node"
	APILockTokens                       string = "/api/lock-tokens"
	APICreateDataToken                  string = "/api/create-data-token"
	APICommitDataToken                  string = "/api/commit-data-token"
	APICheckDataToken                   string = "/api/check-data-token"
	APIGetDataToken                     string = "/api/get-data-token"
	APISetupDB                          string = "/api/setup-db"
	APIGetTxnByTxnID                    string = "/api/get-by-txnId"
	APIGetTxnByDID                      string = "/api/get-by-did"
	APIGetTxnByComment                  string = "/api/get-by-comment"
	APIGetAllNFT                        string = "/api/list-nfts"
	APIAddNFTSale                       string = "/api/addnftsale"
	APIDeploySmartContract              string = "/api/deploy-smart-contract"
	APIExecuteSmartContract             string = "/api/execute-smart-contract"
	APIFetchSmartContract               string = "/api/fetch-smart-contract"
	APIPublishContract                  string = "/api/publish-smart-contract"
	APISubscribecontract                string = "/api/subscribe-smart-contract"
	APIDumpSmartContractTokenChainBlock string = "/api/dump-smart-contract-token-chain"
	APIGetSmartContractTokenData        string = "/api/get-smart-contract-token-chain-data"
	APIRegisterCallBackURL              string = "/api/register-callback-url"
	APIGetTxnByNode                     string = "/api/get-by-node"
	APIRemoveTokenChainBlock            string = "/api/remove-token-chain-block"
	APIPeerID                           string = "/api/get-peer-id"
	APIReleaseAllLockedTokens           string = "/api/release-all-locked-tokens"
	APICheckQuorumStatus                string = "/api/check-quorum-status"
	APIAddPeerDetails                   string = "/api/add-peer-details"
	APIGetPledgedTokenDetails           string = "/api/get-pledgedtoken-details"
	APICheckPinnedState                 string = "/api/check-pinned-state"
	APISelfTransfer                     string = "/api/initiate-self-transfer"
	APIRunUnpledge                      string = "/api/run-unpledge"
	APIUnpledgePOWPledgeTokens          string = "/api/unpledge-pow-unpledge-tokens"
	APIInitiatePinRBT                   string = "/api/initiate-pin-token"
	APIValidateTokenChain               string = "/api/validate-token-chain"
	APIGenerateFaucetTestToken          string = "/api/generate-faucettest-token"
	APIFaucetTokenCheck                 string = "/api/faucet-token-check"
	APIDumpFTTokenChainBlock            string = "/api/dump-ft-token-chain"
	APIInitiateFTTransfer               string = "/api/initiate-ft-transfer"
	APIFixFTCreator                     string = "/api/fix-ft-creator"
	APIGetFTCreatorStats                string = "/api/get-ft-creator-stats"
	APIValidateToken                    string = "/api/validate-token"
	APIAddUserAPIKey                    string = "/api/add-user-api-key"
	APIDeployNFT                        string = "/api/deploy-nft"
	APIExecuteNFT                       string = "/api/execute-nft"
	APIDumpNFTTokenChain                string = "/api/dump-nft-token-chain"
	APISubscribeNFT                     string = "/api/subscribe-nft"
	APIGetNFTTokenChainData             string = "/api/get-nft-token-chain-data"
	APIFetchNft                         string = "/api/fetch-nft"
	APIGetFTTokenchain                  string = "/api/get-ft-token-chain"
	APISendJWTFromWallet                string = "/api/send-jwt-from-wallet"
	APIAddPeerDetailsFromExplorer       string = "/api/add-peer-details-from-explorer"
	APIGetFTTxnByDID                    string = "/api/get-ft-txn-by-did"
	APIUpdateTokenStatus                string = "/api/update-token-status"
	APIGetTokenStatus                   string = "/api/get-token-status"
	APIMigrateFTTransactions            string = "/api/migrate-ft-transactions"
	APIGetFTMigrationStatus             string = "/api/get-ft-migration-status"
	APIRetryFailedFTDownloads           string = "/api/retry-failed-ft-downloads"
	APIGetFailedFTDownloadStatus        string = "/api/get-failed-ft-download-status"
	APIRecoverLostTokens                string = "/api/recover-lost-tokens"
	APIRemoteRecoverTokens              string = "/api/remote-recover-tokens"

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

	//Below are Explorer-service API endpoints

	APINotifyDeExpBlockUpdate string = "/api/block-update"
	APINotifyDeExpTokenUpdate string = "/api/token-update"

	APIRemoveStaleDID string = "/api/remove-stale-did"

	APIGenerateSmartContract string = "/rubix/v1/smart_contracts/generate"
	APIListSmartContracts    string = "/rubix/v1/smart_contracts"
	APIGetSmartContractChain string = "/rubix/v1/smart_contracts/{contract_id}/chain"
	APICreateNFT             string = "/rubix/v1/nfts/generate"
	APIListNFTs              string = "/rubix/v1/nfts"
	APIGetNFTChain           string = "/rubix/v1/nfts/{nft_id}/chain"
	APITransaction           string = "/rubix/v1/tx"
	APIGetTransactionByID    string = "/rubix/v1/tx/{tx_id}"

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
