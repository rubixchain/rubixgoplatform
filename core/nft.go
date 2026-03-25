package core

// nft.go — dead-code stub (Phase 09 replacement target)
// NFT creation, deployment, and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// NFTReq holds the inputs for creating a new NFT.
type NFTReq struct {
	DID      string
	Metadata string
	Artifact string
	NFTPath  string
}

// NFTIpfsInfo holds IPFS-related info for an NFT.
type NFTIpfsInfo struct {
	DID          string
	ArtifactHash string
}

// FetchNFTRequest is the request payload for fetching an NFT.
type FetchNFTRequest struct {
	NFT     string
	NFTPath string
}

// CreateNFTRequest stubs NFT creation; replaced by InitiateTransaction in Phase 09.
func (c *Core) CreateNFTRequest(requestID string, createNFTRequest NFTReq) {
	br := model.BasicResponse{
		Status:  false,
		Message: "CreateNFT not yet implemented",
	}
	ch := c.GetWebReq(requestID)
	if ch == nil {
		c.log.Error("CreateNFTRequest: failed to get did channels")
		return
	}
	ch.OutChan <- &br
}

// DeployNFT stubs NFT deployment.
func (c *Core) DeployNFT(reqID string, deployReq model.DeployNFTRequest) {
	br := model.BasicResponse{
		Status:  false,
		Message: "DeployNFT not yet implemented",
	}
	ch := c.GetWebReq(reqID)
	if ch == nil {
		c.log.Error("DeployNFT: failed to get did channels")
		return
	}
	ch.OutChan <- &br
}

// GetAllNFT returns an empty NFT list stub.
func (c *Core) GetAllNFT() model.NFTList {
	return model.NFTList{}
}

// GetNFTsByDid returns an empty NFT list stub for a given DID.
func (c *Core) GetNFTsByDid(did string) model.NFTList {
	return model.NFTList{}
}

// ExecuteNFT stubs NFT execution.
func (c *Core) ExecuteNFT(reqID string, executeReq *model.ExecuteNFTRequest) {
	br := model.BasicResponse{
		Status:  false,
		Message: "ExecuteNFT not yet implemented",
	}
	ch := c.GetWebReq(reqID)
	if ch == nil {
		c.log.Error("ExecuteNFT: failed to get did channels")
		return
	}
	ch.OutChan <- &br
}

// SubscribeNFTSetup stubs NFT subscription setup.
func (c *Core) SubscribeNFTSetup(requestID string, topic string) error {
	return fmt.Errorf("SubscribeNFTSetup: not implemented")
}

// FetchNFT stubs NFT fetching from IPFS.
func (c *Core) FetchNFT(fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	return &model.BasicResponse{
		Status:  false,
		Message: "FetchNFT not yet implemented",
	}
}

// CheckNFTFolderExists stubs NFT folder existence check.
func (c *Core) CheckNFTFolderExists(nft string) (string, error) {
	return "", fmt.Errorf("CheckNFTFolderExists: not implemented")
}

// GetAllNFTs stubs listing all NFTs.
func (c *Core) GetAllNFTs() ([]models.FullNodeNFT, error) {
	return nil, fmt.Errorf("GetAllNFTs: not implemented")
}

// GetNFTChain stubs retrieval of NFT token chain data.
func (c *Core) GetNFTChain(nftID string) ([]models.TokenChainResponse, error) {
	return nil, fmt.Errorf("GetNFTChain: not implemented")
}

// DumpTokenChain stubs a token chain dump operation.
func (c *Core) DumpTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{}
}
