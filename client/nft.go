package client

import (
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

func (c *Client) SubscribeNFT(nft string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	// Use query parameter instead of JSON body
	query := make(map[string]string)
	query["nft"] = nft
	err := c.sendJSONRequest("POST", setup.APISubscribeNFT, query, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
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
