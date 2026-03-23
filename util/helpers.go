package util

import (
	"os"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

func FileWrite(fileName string, data []byte) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

// SanitizeDirPath will check for proper directory path
func SanitizeDirPath(path string) string {
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		return path
	} else {
		return path + "/"
	}
}

// ParseAddress will parse the address and split into Peer ID  & DID
func ParseAddress(addr string) (string, string, bool) {
	peerID := ""
	did := ""
	// check if addr contains the peer ID
	if strings.Contains(addr, ".") {
		str := strings.Split(addr, ".")
		if len(str) != 2 {
			return "", "", false
		}
		peerID = str[0]
		did = str[1]
	} else {
		did = addr
	}
	//TODO:: Validation
	return peerID, did, true
}

func HasNFTOwnershipTransfer(nfts []models.NFTInfo) bool {
	for _, nft := range nfts {
		if nft.NFTId == "" {
			continue
		}

		if nft.TransferOwnerShip {
			return true
		}
	}
	return false
}
