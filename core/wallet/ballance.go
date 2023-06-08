package wallet

import "fmt"

//TODO:Change function for part tokens

func (w *Wallet) GetBalance(did string, peerID string) float64 {
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return 0.0
	}
	var count float64
	for _, at := range t {
		if at.TokenStatus == TokenIsFree {
			count++
		}
	}
	fmt.Println(count)
	return count
}
