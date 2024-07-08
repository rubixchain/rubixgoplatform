package command

import (
	"fmt"
	"strings"
)

func (cmd *Command) getTxnDetails() {
	if cmd.did == "" && cmd.txnID == "" && cmd.transComment == "" {
		cmd.log.Error("Please provide did or transaction id or transaction comment to get transaction details")
		return
	}
	if cmd.did != "" && !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) < 59 {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.txnID != "" {
		res, err := cmd.c.GetTxnByID(cmd.txnID)
		if err != nil {
			cmd.log.Error("Invalid response from the node", "err", err)
			return
		}
		if !res.BasicResponse.Status {
			cmd.log.Error("Failed to get Txn details for TxnID", cmd.txnID, " err", err)
		}
		for i := range res.TxnDetails {
			td := res.TxnDetails[i]
			fmt.Printf("%+v", td)
		}
	}

	if cmd.did != "" {
		if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) < 59 {
			cmd.log.Error("Invalid DID")
			return
		}
		res, err := cmd.c.GetTxnByDID(cmd.did, cmd.role)
		if err != nil {
			cmd.log.Error("Invalid response from the node", "err", err)
			return
		}
		if !res.BasicResponse.Status {
			cmd.log.Error("Failed to get Txn details for Did", cmd.did, " err", err)
		}
		for i := range res.TxnDetails {
			td := res.TxnDetails[i]
			fmt.Printf("%+v", td)
		}
	}

	if cmd.transComment != "" {
		res, err := cmd.c.GetTxnByComment(cmd.transComment)
		if err != nil {
			cmd.log.Error("Invalid response from the node", "err", err)
			return
		}
		if !res.BasicResponse.Status {
			cmd.log.Error("Failed to get Txn details for comment", cmd.transComment, " err", err)
		}
		for i := range res.TxnDetails {
			td := res.TxnDetails[i]
			fmt.Printf("%+v", td)
		}
	}
}
