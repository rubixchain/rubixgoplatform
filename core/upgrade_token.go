package core

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// UpgradeRequest represents the request for upgrading tokens.
type UpgradeRequest struct {
	DIDType   int    `json:"did_type"`
	PrivPWD   string `json:"priv_pwd"`
	QuorumPWD string `json:"quorum_pwd"`
	DID       string `json:"did"`
}

// UpgradeTokens upgrades tokens based on the provided upgrade request.
func (c *Core) UpgradeTokens(upgradeRequest *UpgradeRequest) (*model.BasicResponse, error) {
	// Get the DID type for the provided DID
	didType, err := c.w.GetDID(upgradeRequest.DID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DID type for %s: %v", upgradeRequest.DID, err)
	}
	fmt.Println("DID Type is ", didType.Type)
	upgradeRequest.DIDType = didType.Type

	// Get all tokens for the provided DID and RBTType
	tokenDetails, err := c.GetAllTokens(upgradeRequest.DID, model.RBTType)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokens: %v", err)
	}

	// Group tokens by status
	tokensByStatus := make(map[int][]string)
	for _, detail := range tokenDetails.TokenDetails {
		tokensByStatus[detail.Status] = append(tokensByStatus[detail.Status], detail.Token)
	}

	// Prepare the response with status and tokens
	statusTokensResponse := make([]model.StatusTokensResponse, 0, len(tokensByStatus)) // Pre-allocate for efficiency
	for status, tokens := range tokensByStatus {
		statusTokensResponse = append(statusTokensResponse, model.StatusTokensResponse{
			Status: status,
			Tokens: tokens,
		})
	}

	// Get the ipfs cat value of each token
	for _, token := range tokenDetails.TokenDetails {
		path := token.Token
		rpt, err := c.ipfs.Cat(path)
		if err != nil {
			c.log.Error("failed to get from ipfs", "err", err, "path", path)
			return nil, err
		}

		data, err := ioutil.ReadAll(rpt)
		if err != nil {
			c.log.Error("failed to read data from IPFS", "err", err, "path", path)
			return nil, err
		}

		fmt.Println("IPFS Cat Value of ", path, " is ", string(data))
	}

	return &model.BasicResponse{
		Status:  true,
		Message: "Tokens retrieved successfully",
		Result:  statusTokensResponse,
	}, nil
}

func (c *Core) AddTokensForTesting() {
	// Add the string "10010010" to IPFS and get the CID
	// Add more values to IPFS and get the CIDs
	values := []string{"0011", "0651000", "0771878", "0021", "10010010", "005500"}
	cids := make([]string, len(values))
	for i, value := range values {
		cid, err := c.ipfs.Add(bytes.NewReader([]byte(value)))
		if err != nil {
			c.log.Error("failed to add to IPFS", "err", err)
		}
		cids[i] = cid
		fmt.Println("CID of the added value", value, "is", cid)
	}
	cid, err := c.ipfs.Add(bytes.NewReader([]byte("10010010")))
	if err != nil {
		c.log.Error("failed to add to IPFS", "err", err)
	}

	fmt.Println("CID of the added string is", cid)
}
