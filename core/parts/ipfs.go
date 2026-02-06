package parts

// import (
// 	"bytes"
// 	"io"

// 	ipfsnode "github.com/ipfs/go-ipfs-api"
// )

// type IPFSOperation interface {
// 	Add(data io.Reader, opts ...ipfsnode.AddOpts) (string, error)
// 	Cat(hash string) (io.ReadCloser, error)
// }

// func IpfsAddString(indexedTokenID string, ipfsClient IPFSOperation) (string, error) {
// 	indexedTokenIDStrBuffer := bytes.NewBufferString(indexedTokenID)

// 	return ipfsClient.Add(indexedTokenIDStrBuffer, ipfsnode.OnlyHash(true), ipfsnode.Pin(false))
// }

// func IpfsCatString(id string, ipfsClient IPFSOperation) (string, error) {
// 	ipfsCatInfo, err := ipfsClient.Cat(id)
// 	if err != nil {
// 		return "", err
// 	}

// 	ipfsCatBytes, err := io.ReadAll(ipfsCatInfo)
// 	if err != nil {
// 		return "", err
// 	}

// 	return string(ipfsCatBytes), nil
// }
