package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func txCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "transaction related subcommands (RBT, Smart Contract)",
		Long:  "transaction related subcommands (RBT, Smart Contract)",
	}

	cmd.AddCommand(
		getTxnDetails(cmdCfg),
		txTokenCommandGroup(cmdCfg),
		txSmartContractCommandGroup(cmdCfg),
		txDataTokenCommandGroup(cmdCfg),
		nftCommandGroup(cmdCfg),
	)

	return cmd
}

func txTokenCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rbt",
		Short: "RBT related subcommands",
		Long:  "RBT related subcommands",
	}

	cmd.AddCommand(
		transferRBTCmd(cmdCfg),
		generateTestRBTCmd(cmdCfg),
		selfTransferRBTCmd(cmdCfg),
	)

	return cmd
}

func txSmartContractCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smart-contract",
		Short: "Smart Contract related subcommands",
		Long:  "Smart Contract related subcommands",
	}

	cmd.AddCommand(
		generateSmartContractTokenCmd(cmdCfg),
		fetchSmartContractCmd(cmdCfg),
		publishContract(cmdCfg),
		subscribeContract(cmdCfg),
		deploySmartcontract(cmdCfg),
		executeSmartcontract(cmdCfg),
	)

	return cmd
}

// DEPRECATED
func txDataTokenCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "data-token",
		Short: "(DEPRECATED) Data Token related subcommands",
		Long: "(DEPRECATED) Data Token related subcommands",
	}

	cmd.AddCommand(
		createDataToken(cmdCfg),
		commitDataToken(cmdCfg),
	)

	return cmd
}

func getTxnDetails(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx-details",
		Short: "Get transaction details",
		Long:  "Get transaction details",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" || cmdCfg.txnID == "" || cmdCfg.transComment == "" {
				cmdCfg.log.Error("Please provide did or transaction id or transaction comment to get transaction details")
				return nil
			}

			if cmdCfg.txnID != "" {
				res, err := cmdCfg.c.GetTxnByID(cmdCfg.txnID)
				if err != nil {
					cmdCfg.log.Error("Invalid response from the node", "err", err)
					return nil
				}
				if !res.BasicResponse.Status {
					cmdCfg.log.Error("Failed to get Txn details for TxnID", cmdCfg.txnID, " err", err)
					return nil
				}
				for i := range res.TxnDetails {
					td := res.TxnDetails[i]
					fmt.Printf("%+v", td)
				}
			}

			if cmdCfg.did != "" {
				res, err := cmdCfg.c.GetTxnByDID(cmdCfg.did, cmdCfg.role)
				if err != nil {
					cmdCfg.log.Error("Invalid response from the node", "err", err)
					return nil
				}
				if !res.BasicResponse.Status {
					cmdCfg.log.Error("Failed to get Txn details for Did", cmdCfg.did, " err", err)
					return nil
				}
				for i := range res.TxnDetails {
					td := res.TxnDetails[i]
					fmt.Printf("%+v", td)
				}
			}

			if cmdCfg.transComment != "" {
				res, err := cmdCfg.c.GetTxnByComment(cmdCfg.transComment)
				if err != nil {
					cmdCfg.log.Error("Invalid response from the node", "err", err)
					return nil
				}
				if !res.BasicResponse.Status {
					cmdCfg.log.Error("Failed to get Txn details for comment", cmdCfg.transComment, " err", err)
					return nil
				}
				for i := range res.TxnDetails {
					td := res.TxnDetails[i]
					fmt.Printf("%+v", td)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.txnID, "txnID", "", "Transaction ID")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")
	cmd.Flags().StringVar(&cmdCfg.role, "role", "", "Sender/Receiver")
	cmd.Flags().StringVar(&cmdCfg.transComment, "transComment", "", "Transaction comment")

	return cmd
}
