package core

import (
	"bytes"
	"io"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
)

// IpfsAddWithBackoff performs IPFS add operation with retry logic
// This is kept for backward compatibility. New code should use c.ipfsOps.Add() directly
func IpfsAddWithBackoff(ipfs *shell.Shell, data io.Reader, opts ...shell.AddOpts) (string, error) {
	var lastErr error
	var dataBytes []byte
	
	// Read data once to allow retries
	if seeker, ok := data.(io.Seeker); ok {
		// If data is seekable, we can rewind it
		defer seeker.Seek(0, io.SeekStart)
	} else {
		// Otherwise, read it all into memory
		dataBytes = readAll(data)
		data = bytes.NewReader(dataBytes)
	}
	
	for try := 0; try < 5; try++ {
		hash, err := ipfs.Add(data, opts...)
		
		if err == nil {
			return hash, nil
		}
		lastErr = err
		
		// Exponential backoff
		backoff := time.Duration(100*(1<<try)) * time.Millisecond
		time.Sleep(backoff)
		
		// Rewind data for retry
		if seeker, ok := data.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		} else {
			data = bytes.NewReader(dataBytes)
		}
	}
	return "", lastErr
}

// helper to clone io.Reader
func readAll(r io.Reader) []byte {
	buf, _ := io.ReadAll(r)
	return buf
}
