// UpgradeTokensClient sends a POST request to upgrade tokens using the provided UpgradeRequest.
// It returns a BasicResponse and an error if any.
// The optional timeout parameter specifies the maximum duration for the request.
package client

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) UpgradeTokensClient(upgradeRequest *core.UpgradeRequest, timeout ...time.Duration) (*model.BasicResponse, error) {
	var responseModel model.BasicResponse
	fmt.Println("Upgrading tokens client side")
	err := c.sendJSONRequest("POST", setup.APIUpgradeTokens, nil, upgradeRequest, &responseModel, timeout...)
	if err != nil {
		return nil, err
	}
	return &responseModel, nil
}
