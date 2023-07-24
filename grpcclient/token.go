package main

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
)

func (cmd *Command) GenerateRBT() {
	cmd.GetDIDAccess()
	gr := &protos.GenerateReq{
		TokenCount: float64(cmd.numTokens),
		Did:        cmd.did,
	}
	br, err := cmd.c.GenerateRBT(cmd.ctx, gr)
	if err != nil {
		fmt.Printf("Failed to generate RBT, %s\n", err.Error())
		return
	}
	stream, err := cmd.c.StreamSignature(cmd.ctx)
	if err != nil {
		fmt.Printf("Failed to generate RBT, %s\n", err.Error())
		return
	}
	defer stream.CloseSend()
	for {
		if !br.SignNeeded {
			fmt.Printf("RBT generated successfully\n")
			return
		}
		resp := &protos.SignResponse{
			ReqID: br.SignRequest.ReqID,
			Mode:  br.SignRequest.Mode,
		}
		switch int(br.SignRequest.Mode) {
		case did.BasicDIDMode:
			resp.Password = cmd.privPWD
		case did.ChildDIDMode:
			resp.Password = cmd.privPWD
		default:
			fmt.Printf("DID mode is not supported, %d\n", br.SignRequest.Mode)
			return
		}
		err = stream.Send(resp)
		if err != nil {
			fmt.Printf("Failed to generate RBT, %s\n", err.Error())
			return
		}
		br, err = stream.Recv()
		if err != nil {
			fmt.Printf("Failed to generate RBT, %s\n", err.Error())
			return
		}
	}
}

func (cmd *Command) GettAllTokens() {

}
