package core

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// GetWallet returns the wallet instance for direct access in dev/test scenarios.
// In production, wallet operations go through Core methods.
func (c *Core) GetWallet() *wallet.Wallet {
	return c.w
}

// DumpSmartContractTokenChain returns token chain blocks for a smart contract token.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (c *Core) DumpSmartContractTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{BasicResponse: model.BasicResponse{Status: false, Message: "not implemented"}}
}

// DumpNFTTokenChain returns token chain blocks for an NFT token.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (c *Core) DumpNFTTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{BasicResponse: model.BasicResponse{Status: false, Message: "not implemented"}}
}

// GetSmartContractTokenChainData returns smart contract execution data from the token chain.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (c *Core) GetSmartContractTokenChainData(req *model.SmartContractTokenChainDataReq) *model.SmartContractDataReply {
	return &model.SmartContractDataReply{BasicResponse: model.BasicResponse{Status: false, Message: "not implemented"}}
}

// GetNFTTokenChainData returns NFT chain data from the token chain.
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (c *Core) GetNFTTokenChainData(req *model.SmartContractTokenChainDataReq) *model.NFTDataReply {
	return &model.NFTDataReply{BasicResponse: model.BasicResponse{Status: false, Message: "not implemented"}}
}

// RemoveTokenChainBlock removes a token chain block (diagnostic/admin operation).
// TODO(phase11-upstream): implement using PostgreSQL tokenchain queries.
func (c *Core) RemoveTokenChainBlock(req *model.TCRemoveRequest) *model.TCRemoveReply {
	return &model.TCRemoveReply{BasicResponse: model.BasicResponse{Status: false, Message: "not implemented"}}
}

// ReleaseAllLockedTokens releases all tokens that are currently locked.
// TODO(phase11-upstream): implement via wallet lock release.
func (c *Core) ReleaseAllLockedTokens() model.BasicResponse {
	return model.BasicResponse{Status: false, Message: "not implemented"}
}

// RetryFailedFTDownloads retries downloading FT tokens that previously failed.
// TODO(phase11-upstream): implement FT retry logic.
func (c *Core) RetryFailedFTDownloads(did string) (string, error) {
	return "not implemented", nil
}

// GetFailedFTDownloadStatus returns the status of previously failed FT downloads.
// TODO(phase11-upstream): implement FT download status tracking.
func (c *Core) GetFailedFTDownloadStatus(did string) (interface{}, error) {
	return nil, nil
}
