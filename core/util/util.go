package util

import (
	"fmt"
	"net"

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
