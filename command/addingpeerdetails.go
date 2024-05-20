package command

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (cmd *Command) AddPeerDetails() {
	var peerID string
	var did string
	var did_type int
	var err error
	if cmd.peerID == "" {
		fmt.Print("Enter PeerID : ")
		_, err = fmt.Scan(&peerID)
		if err != nil {
			cmd.log.Error("Failed to get PeerID")
			return
		}

	} else {
		peerID = cmd.peerID
	}

	if !strings.HasPrefix(peerID, "12D3KooW") || len(peerID) < 52 {
		cmd.log.Error("Invalid PeerID")
		return
	}

	if cmd.did == "" {
		fmt.Print("Enter DID : ")
		_, err = fmt.Scan(&did)
		if err != nil {
			cmd.log.Error("Failed to get DID")
			return
		}
	} else {
		did = cmd.did
	}
	if !strings.HasPrefix(did, "bafybmi") || len(did) < 59 {
		cmd.log.Error("Invalid DID")
		return
	}

	fmt.Print("Enter DID Type : ")
	_, err = fmt.Scan(&did_type)
	if err != nil {
		cmd.log.Error("Invalid DID Type")
		return
	}
	if did_type < 0 || did_type > 4 {
		cmd.log.Error("DID should be between 0 and 4")
		return
	}

	peer_detail := wallet.DIDPeerMap{
		PeerID:  peerID,
		DID:     did,
		DIDType: &did_type,
	}
	msg, status := cmd.c.AddPeer(&peer_detail)
	if !status {
		cmd.log.Error("Failed to add peer in DB", "message", msg)
		return
	}
	cmd.log.Info("Peer added successfully")
}
