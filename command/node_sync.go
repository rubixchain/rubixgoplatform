package command

import "fmt"

func (cmd *Command) nodeSync() {
	if cmd.did == "" {
		cmd.log.Error("DID is required to get its token information")
		return
	}
	cmd.log.Info(fmt.Sprintf("Node sync for DID %v has started", cmd.did))

	nodeSyncResponse, err := cmd.c.NodeSync(cmd.did)
	if err != nil {
		cmd.log.Error(fmt.Sprintf("Failed to sync node for DID %v, err: %v", cmd.did, err))
		return
	}
	if !nodeSyncResponse.Status {
		cmd.log.Error(fmt.Sprintf("Failed to sync node for DID %v, cause: %v", cmd.did, nodeSyncResponse.Message))
		return
	}
	cmd.log.Info(fmt.Sprintf("Node sync for DID %v has been completed successfully", cmd.did))
}
