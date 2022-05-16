package util

import (
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/sha3"
)

func CalculateHash(data []byte, method string) []byte {
	switch method {
	case "SHA3-256":
		h := sha3.New256()
		h.Write(data)
		return h.Sum(nil)
	default:
		return nil
	}
}

func GetMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, i := range interfaces {
		ha := i.HardwareAddr
		return fmt.Sprintf("%x", ha)
	}
	return ""
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
