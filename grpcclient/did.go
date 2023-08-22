package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (cmd *Command) CreateDID() {
	req := &protos.CreateDIDReq{
		DidMode:    int32(cmd.didType),
		RootDID:    cmd.didRoot,
		MasterDID:  cmd.did,
		PrivKeyPwd: cmd.privPWD,
		Secret:     cmd.didSecret,
	}
	if cmd.didType == did.BasicDIDMode || cmd.didType == did.StandardDIDMode {
		ib, err := ioutil.ReadFile(cmd.imgFile)
		if err != nil {
			fmt.Printf("Invalid image file, %s\n", err.Error())
			return
		}
		req.DidImage = base64.StdEncoding.EncodeToString(ib)
	}
	resp, err := cmd.c.CreateDID(cmd.ctx, req)

	if err != nil {
		fmt.Printf("faield to create did, %s\n", err.Error())
		return
	}
	fmt.Printf("DID created : %s\n", resp.Did)
}

func (cmd *Command) GetDIDAccess() {
	ar := &protos.AccessReq{
		Did:      cmd.did,
		Password: cmd.privPWD,
	}
	resp, err := cmd.c.GetDIDAccess(cmd.ctx, ar)

	if err != nil {
		fmt.Printf("faield to get access to did, %s\n", err.Error())
		return
	}
	fmt.Printf("Got the DID access\n")
	cmd.accessToken = resp.AccessToken
	cmd.ctx = metadata.AppendToOutgoingContext(cmd.ctx, "authorization", "Bearer "+resp.AccessToken)
}

func (cmd *Command) GetBalance() {
	cmd.GetDIDAccess()
	resp, err := cmd.c.GetBalance(cmd.ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Printf("faield to get did balance, %s\n", err.Error())
		return
	}
	fmt.Printf("DID balance : %f\n", resp.Balance)
}
