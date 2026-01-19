package parts

import (
	"bytes"
	"io"

	ipfsnode "github.com/ipfs/go-ipfs-api"
)

type IPFSOperation interface {
	Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error)
	Cat(hash string) (io.ReadCloser, error)
}

func IpfsAddString(heirarchicalID TokenID, ipfsClient IPFSOperation) (string, error) {
	heirarchicalIDStr := heirarchicalID.String()
	heirarchicalIDStrBuffer := bytes.NewBufferString(heirarchicalIDStr)

	return ipfsClient.Add(heirarchicalIDStrBuffer, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
}

func IpfsCatString(id string, ipfsClient IPFSOperation) (TokenID, error) {
	ipfsCatInfo, err := ipfsClient.Cat(id)
	if err != nil {
		return "", err
	}

	ipfsCatBytes, err := io.ReadAll(ipfsCatInfo)
	if err != nil {
		return "", err
	}

	return TokenID(ipfsCatBytes), nil
}
