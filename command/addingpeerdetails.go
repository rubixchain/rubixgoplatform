package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (cmd *Command) AddPeerDetails() {
	var peerID string
	var did string
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
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.peerID)
	if !strings.HasPrefix(cmd.peerID, "12D3KooW") || len(cmd.peerID) != 52 || !is_alphanumeric {
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
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}

	// did_type = cmd.didType
	if cmd.didType < 0 || cmd.didType > 4 {
		cmd.log.Error("DID Type should be between 0 and 4")
		return
	}

	peer_detail := wallet.DIDPeerMap{
		PeerID:  peerID,
		DID:     did,
		DIDType: &cmd.didType,
	}
	msg, status := cmd.c.AddPeer(&peer_detail)
	if !status {
		cmd.log.Error("Failed to add peer in DB", "message", msg)
		return
	}
	cmd.log.Info("Peer added successfully")
}
