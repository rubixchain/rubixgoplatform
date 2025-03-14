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
	APICreateDID                        string = "/api/createdid"
	APIGetAllDID                        string = "/api/getalldid"
	APIGetAllTokens                     string = "/api/getalltokens"
	APIAddQuorum                        string = "/api/addquorum"
	APIGetAllQuorum                     string = "/api/getallquorum"
	APIRemoveAllQuorum                  string = "/api/removeallquorum"
	APISetupQuorum                      string = "/api/setup-quorum"
	APISetupService                     string = "/api/setup-service"
	APIGenerateTestToken                string = "/api/generate-test-token"
	APIInitiateRBTTransfer              string = "/api/initiate-rbt-transfer"
	APIGetAccountInfo                   string = "/api/get-account-info"
	APISignatureResponse                string = "/api/signature-response"
	APIDumpTokenChainBlock              string = "/api/dump-token-chain"
	APIRegisterDID                      string = "/api/register-did"
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
	APICreateNFT                        string = "/api/create-nft"
	APIGetAllNFT                        string = "/api/list-nfts"
	APIAddNFTSale                       string = "/api/addnftsale"
	APIDeploySmartContract              string = "/api/deploy-smart-contract"
	APIExecuteSmartContract             string = "/api/execute-smart-contract"
	APIGenerateSmartContract            string = "/api/generate-smart-contract"
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
	APIGetAllExplorer                   string = "/api/get-all-explorer"
	APIAddExplorer                      string = "/api/add-explorer"
	APIRemoveExplorer                   string = "/api/remove-explorer"
	APIAddPeerDetails                   string = "/api/add-peer-details"
	APIGetPledgedTokenDetails           string = "/api/get-pledgedtoken-details"
	APICheckPinnedState                 string = "/api/check-pinned-state"
	APISelfTransfer                     string = "/api/initiate-self-transfer"
	APIRunUnpledge                      string = "/api/run-unpledge"
	APIUnpledgePOWPledgeTokens          string = "/api/unpledge-pow-unpledge-tokens"
	APIInitiatePinRBT                   string = "/api/initiate-pin-token"
	APIRecoverRBT                       string = "/api/recover-token"
	APIValidateTokenChain               string = "/api/validate-token-chain"
	APIGenerateFaucetTestToken          string = "/api/generate-faucettest-token"
	APIFaucetTokenCheck                 string = "/api/faucet-token-check"
	APICreateFT                         string = "/api/create-ft"
	APIDumpFTTokenChainBlock            string = "/api/dump-ft-token-chain"
	APIInitiateFTTransfer               string = "/api/initiate-ft-transfer"
	APIGetFTInfo                        string = "/api/get-ft-info-by-did"
	APIValidateToken                    string = "/api/validate-token"
	APIAddUserAPIKey                    string = "/api/add-user-api-key"
	APIDeployNFT                        string = "/api/deploy-nft"
	APIExecuteNFT                       string = "/api/execute-nft"
	APIDumpNFTTokenChain                string = "/api/dump-nft-token-chain"
	APISubscribeNFT                     string = "/api/subscribe-nft"
	APIGetNFTTokenChainData             string = "/api/get-nft-token-chain-data"
	APIFetchNft                         string = "/api/fetch-nft"
	APIGetNftsByDid                     string = "/api/get-nfts-by-did"
	APIGetFTTokenchain                  string = "/api/get-ft-token-chain"
	APIRequestDIDForPubKey              string = "/api/request-did-for-pubkey"
	APISendJWTFromWallet                string = "/api/send-jwt-from-wallet"
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
