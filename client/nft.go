package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

type CreateNFTReq struct {
	DID      string
	Metadata string
	Artifact string
}

type FetchNFTRequest struct {
	NFT     string
	NFTPath string
}

func (c *Client) CreateNFT(createNFTReq *CreateNFTReq) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	files := make(map[string]string)
	if createNFTReq.DID != "" {
		fields["did"] = createNFTReq.DID
	}
	if createNFTReq.Metadata != "" {
		files["metadata"] = createNFTReq.Metadata
	}

	if createNFTReq.Artifact != "" {
		files["artifact"] = createNFTReq.Artifact
	}
	//To add more than 1 file : Tobe done
	// for _, fn := range nt.Files {
	// 	fuid := path.Base(fn)
	// 	files[fuid] = fn
	// }
	var br model.BasicResponse
	err := c.sendMutiFormRequest("POST", setup.APICreateNFT, nil, fields, files, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) ExecuteNFT(transferRequest *model.ExecuteNFTRequest) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIExecuteNFT, nil, transferRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Transfer NFT", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}

func (c *Client) DeployNFT(deployRequest *model.DeployNFTRequest) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIDeployNFT, nil, deployRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Deploy NFT", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}

func (c *Client) SubscribeNFT(nft string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	newSubscription := model.NewNFTSubscription{
		NFT: nft,
	}
	err := c.sendJSONRequest("POST", setup.APISubscribeNFT, nil, &newSubscription, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetAllNFTs(did string) (*model.NFTTokens, error) {
	q := make(map[string]string)
	q["did"] = did
	var tkns model.NFTTokens
	err := c.sendJSONRequest("GET", setup.APIGetAllNFT, q, nil, &tkns)
	if err != nil {
		return nil, err
	}
	return &tkns, nil
}

func (c *Client) FetchNFT(fetchNft *FetchNFTRequest) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	if fetchNft.NFT != "" {
		fields["nft"] = fetchNft.NFT
	}

	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIFetchNft, fields, nil, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil

}
