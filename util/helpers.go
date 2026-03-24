package util

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// GetRandBytes generates n cryptographically random bytes.
// Returns nil on error (callers treat nil as a fatal initialisation failure).
func GetRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil
	}
	return b
}

// Filecopy copies the file at src to dst, creating dst if necessary.
// Returns the number of bytes written.
func Filecopy(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	n, err := io.Copy(out, in)
	return n, err
}

// DirCopy recursively copies the directory tree rooted at src into dst.
func DirCopy(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		_, err = Filecopy(path, target)
		return err
	})
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
