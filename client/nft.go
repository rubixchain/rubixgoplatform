package client

import (
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
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

func (c *Client) CreateNFT(createNFTReq *CreateNFTReq) (*models.BasicResponse, error) {
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
	var br models.BasicResponse
	err := c.sendMutiFormRequest("POST", setup.APICreateNFT, nil, fields, files, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) SubscribeNFT(nft string) (*models.BasicResponse, error) {
	var response models.BasicResponse
	// Use query parameter instead of JSON body
	query := make(map[string]string)
	query["nft"] = nft
	err := c.sendJSONRequest("POST", setup.APISubscribeNFT, query, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetNFTsByDid(did string) (*models.BasicResponse, error) {
	pathParams := make(map[string]string)
	pathParams["did"] = did
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetNftByDid, pathParams)
	if err != nil {
		return nil, err
	}
	var nftResp models.BasicResponse
	err = c.sendJSONRequest("GET", endpoint, nil, nil, &nftResp)
	if err != nil {
		return nil, err
	}
	return &nftResp, nil
}

func (c *Client) FetchNFT(fetchNft *FetchNFTRequest) (*models.BasicResponse, error) {
	fields := make(map[string]string)
	if fetchNft.NFT != "" {
		fields["nft"] = fetchNft.NFT
	}

	var basicResponse models.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIFetchNft, fields, nil, &basicResponse)
	if err != nil {
		return nil, err
	}
	return &basicResponse, nil

}
