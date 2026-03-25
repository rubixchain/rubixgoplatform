package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
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

func DirCopy(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = os.MkdirAll(dstfp, srcinfo.Mode())
			if err != nil {
				return err
			}
		} else {
			if _, err = Filecopy(srcfp, dstfp); err != nil {
				return err
			}
		}
	}
	return nil
}

func Filecopy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
