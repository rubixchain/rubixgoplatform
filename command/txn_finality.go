package command

import "fmt"

func (cmd *Command) getPendingTxn() {
	res, err := cmd.c.GetPendingTxn()
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !res.BasicResponse.Status {
		cmd.log.Error("Failed to get Txn details", err)
	}
	for i := range res.TxnIds {
		fmt.Println("/n Txn ID : " + res.TxnIds[i])
	}
}

func (cmd *Command) initiateTxnFinality() {
	res, err := cmd.c.InitiateTxnFinality(cmd.txnID)
	if err != nil {
		cmd.log.Error("Failed to achieve Finality", err)
	}
	fmt.Printf("%+v", res)
}
