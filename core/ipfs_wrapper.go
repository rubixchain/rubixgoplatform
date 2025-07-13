package core

import (
	"bytes"
	"context"
	"io"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
)

var ipfsAddSem = make(chan struct{}, 10) // up to 10 concurrent requests

// IpfsAddWithBackoff is a legacy function that uses the old semaphore approach
// It's kept for backward compatibility but new code should use the health manager
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

// IpfsAddWithHealthCheck is the new function that uses the health manager
// This should be used instead of IpfsAddWithBackoff for better reliability
func IpfsAddWithHealthCheck(core *Core, data io.Reader, opts ...shell.AddOpts) (string, error) {
	var result string
	var operationErr error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := core.ipfsHealth.ExecuteWithHealthCheck(ctx, func() error {
		hash, err := core.ipfs.Add(data, opts...)
		if err != nil {
			operationErr = err
			return err
		}
		result = hash
		return nil
	})

	if err != nil {
		return "", err
	}

	return result, operationErr
}

// helper to clone io.Reader
func readAll(r io.Reader) []byte {
	buf, _ := io.ReadAll(r)
	return buf
}
