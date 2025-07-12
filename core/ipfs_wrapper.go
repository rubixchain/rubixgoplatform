package core

import (
	"bytes"
	"io"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
)

var ipfsAddSem = make(chan struct{}, 10) // up to 10 concurrent requests

func IpfsAddWithBackoff(ipfs *shell.Shell, data io.Reader, opts ...shell.AddOpts) (string, error) {
	var lastErr error
	for try := 0; try < 5; try++ {
		ipfsAddSem <- struct{}{} // acquire slot
		hash, err := ipfs.Add(data, opts...)
		<-ipfsAddSem // release slot

		if err == nil {
			return hash, nil
		}
		lastErr = err
		backoff := time.Duration(100*(1<<try)) * time.Millisecond
		time.Sleep(backoff)
		data = bytes.NewReader(readAll(data)) // rewind buffer
	}
	return "", lastErr
}

// helper to clone io.Reader
func readAll(r io.Reader) []byte {
	buf, _ := io.ReadAll(r)
	return buf
}
