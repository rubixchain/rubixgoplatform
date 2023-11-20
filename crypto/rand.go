package crypto

import "io"

// GetRandBytes will generate rand bytes
func GetRandBytes(reader io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := reader.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}
