package util

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func PublishUnpledgeInfo(pubsub *types.PubSub, eventUnpledgeInfo *models.EventUnpledgeInfo) error {
	if len(eventUnpledgeInfo.UnpledgeInfo) == 0 {
		return fmt.Errorf("PublishUnpledgeInfo: no unpledge info to publish")
	}

	if eventUnpledgeInfo.UnpledgeTransactionID == "" {
		return fmt.Errorf("PublishUnpledgeInfo: unpledge transaction ID is required")
	}

	if eventUnpledgeInfo.PledgeTransactionID == "" {
		return fmt.Errorf("PublishUnpledgeInfo: pledge transaction ID is required")
	}

	if err := pubsub.Publish(constants.Event_RubixTxns, eventUnpledgeInfo); err != nil {
		return fmt.Errorf("PublishUnpledgeInfo: failed to publish unpledge info, err: %v", err)
	}

	return nil
}
