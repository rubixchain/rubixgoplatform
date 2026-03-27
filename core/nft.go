package core

// nft.go — dead-code stub (Phase 09 replacement target)
// NFT creation, deployment, and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
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

func (c *Core) CreateNFTRequest(requestID string, createNFTRequest NFTReq) {
	defer os.RemoveAll(createNFTRequest.NFTPath)
	createNFTResponse := c.createNFT(requestID, createNFTRequest)
	didChannel := c.GetWebReq(requestID)
	if didChannel == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
	}
	didChannel.OutChan <- createNFTResponse
}

func (c *Core) createNFT(requestID string, createNFTRequest NFTReq) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}
	nftFolderHash, err := c.ipfsOps.AddDir(createNFTRequest.NFTPath)
	if err != nil {
		c.log.Error("Failed to add nft file to IPFS", "err", err)
		return basicResponse
	}
	nft := NFTIpfsInfo{
		DID:          createNFTRequest.DID,
		ArtifactHash: nftFolderHash,
	}

	nftJSON, err := json.MarshalIndent(nft, "", "  ")
	if err != nil {
		c.log.Error("Failed to marshal nft struct", "err", err)
		return basicResponse
	}

	nftHash, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(nftJSON))
	if err != nil {
		c.log.Error("Failed to add nft to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("The NFT token hash generated ", nftHash)

	// Set the response status and message
	nftTokenResponse := &SmartContractTokenResponse{
		Message: "NFT Token generated successfully",
		Result:  nftHash,
	}

	_, err = c.RenameNFTFolder(createNFTRequest.NFTPath, nftHash)
	if err != nil {
		c.log.Error("Failed to rename NFT folder", "err", err)
		return basicResponse
	}

	basicResponse.Status = true
	basicResponse.Message = nftTokenResponse.Message
	basicResponse.Result = nftTokenResponse.Result

	return basicResponse
}

func (c *Core) DeployNFT(reqID string, deployReq model.DeployNFTRequest) {
	br := c.deployNFT(reqID, deployReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) deployNFT(reqID string, deployReq model.DeployNFTRequest) *model.BasicResponse {
	// st := time.Now()
	// txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(deployReq.DID)
	if !ok {
		resp.Message = "Invalid Deployer DID"
		return resp
	}

	// isNFT, err := c.w.IsNFT(deployReq.NFT)
	// if err != nil {
	// 	resp.Message = "deployNFT : The TokenID given is not an NFT"
	// 	return resp
	// }
	// if !isNFT {
	// 	resp.Message = "deployNFT : The TokenID given is not an NFT"
	// 	return resp
	// }

	// This part need to be verified.
	//Here we are querying the db and checkign whether the NFT has already been deployed or not.
	// Need to ensure whether this itself is the proper approach : This was the approach we were doing previously
	_, err := c.w.GetTokenByTokenID(deployReq.NFT)
	if err == nil {
		c.log.Error(fmt.Sprintf("NFT %v has been already been deployed", deployReq.NFT))
		resp.Message = fmt.Sprintf("NFT %v has already been deployed", deployReq.NFT)
		return resp
	}

	_, err = c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Deployer DID of the NFT deployer, " + err.Error()
		return resp
	}

	// Building of the TransactionInfo and

	nftTokenDetails := wallet.NFT{
		TokenID:     deployReq.NFT,
		DID:         deployReq.DID,
		TokenStatus: wallet.TokenIsFree,
		TokenValue:  floatPrecision(deployReq.NFTValue, MaxDecimalPlaces),
		Metadata:    deployReq.NFTMetadata,
		Filename:    deployReq.NFTFileName,
	}

	if err := c.w.CreateNFT(&nftTokenDetails, false); err != nil {
		c.log.Error("Failed to write nft to storage in NFTTokenStorage", err)
		return resp
	}

	newEvent := model.NFTEvent{
		NFT:         nftTokenDetails.TokenID,
		ExecutorDid: nftTokenDetails.DID,
		NFTMetadata: nftTokenDetails.Metadata,
		NFTFileName: nftTokenDetails.Filename,
		NFTValue:    nftTokenDetails.TokenValue,
	}

	err = c.publishNewNftEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish NFT info")
	}

	c.log.Info("NFT Deployed successfully")
	resp.Status = true
	msg := fmt.Sprintf("NFT Deployed successfully")
	resp.Message = msg
	return resp
}

// GetAllNFT returns an empty NFT list stub.
func (c *Core) GetAllNFT() model.NFTList {
	return model.NFTList{}
}

// GetNFTsByDid returns an empty NFT list stub for a given DID.
func (c *Core) GetNFTsByDid(did string) model.NFTList {
	return model.NFTList{}
}

func (c *Core) SubscribeNFTSetup(requestID string, topic string) error {
	reqID = requestID
	err := c.ps.SubscribeTopic(topic, c.NFTCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe NFT", topic)
	}
	c.log.Info("Subscribing NFT " + topic + " is successful")
	return err
}

func (c *Core) NFTCallBack(peerID string, topic string, data []byte) {
	var newEvent model.NFTEvent
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to get nft details", "err", err)
		return
	}
	c.log.Info("Recieved Update on nft " + newEvent.NFT)

	nft := newEvent.NFT

	// Fetch NFT files
	var fetchNFT FetchNFTRequest
	fetchNFT.NFT = nft

	fetchNFTResponse := c.FetchNFT(&fetchNFT)
	if !fetchNFTResponse.Status {
		c.log.Error("failed to fetch NFT: ", fetchNFTResponse.Message)
		return
	}

	executorDid := newEvent.ExecutorDid
	publisherAddress := peerID + "." + executorDid
	publisherPeer, err := c.getPeer(publisherAddress)
	if err != nil {
		c.log.Error(fmt.Sprintf("failed to get peer: %v, err: %v", peerID, err))
		return
	}

	err, _ = c.syncTransactionChainFrom(publisherPeer, "", nft)
	if err != nil {
		c.log.Error("Failed to sync token chain block", "err", err)
		return
	}

	// The updation of the db logic if required needs to be added
	c.log.Info("Token chain of " + nft + " syncing successful")
}

func (c *Core) FetchNFT(fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	nftJSON, err := c.ipfsOps.Cat(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("Failed to get NFT from network", "err", err)
		basicResponse.Message = "Failed to get NFT details from network"
		return basicResponse
	}
	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("Failed to read NFT from network", "err", err)
		basicResponse.Message = "Failed to read NFT from network"
		return basicResponse
	}
	nftJSON.Close()

	var nft NFTIpfsInfo
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
		basicResponse.Message = "Failed to parse nft"
		return basicResponse
	}
	err = c.GetNFTFromIpfs(fetchNFTRequest.NFT, nft.ArtifactHash)
	if err != nil {
		c.log.Error("failed to fetch NFT files from IPFS", "err", err)
		basicResponse.Message = "Failed to fetch NFT files from IPFS"
		return basicResponse
	}
	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "NFT fetched successfully"
	basicResponse.Result = &nft

	return basicResponse
}

func (c *Core) publishNewNftEvent(newEvent *model.NFTEvent) error {
	topic := newEvent.NFT
	if c.ps != nil {
		err := c.ps.Publish(topic, newEvent)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
		c.log.Info("New state published on NFT " + topic)
	}
	return nil
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
