package util

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
)

// SignRecoveryChainsNotice signs notice with dc, the committer DID's handle.
func SignRecoveryChainsNotice(dc types.DIDCrypto, notice *types.RecoveryChainsNotice) error {
	if dc == nil || notice == nil || notice.CommitterDID != dc.GetDID() {
		return fmt.Errorf("recovery chains notice: committer DID does not match the DID handle")
	}
	hash, err := recoveryChainsNoticeHash(*notice)
	if err != nil {
		return err
	}
	signature, err := dc.Sign(hash)
	if err != nil {
		return fmt.Errorf("recovery chains notice: sign: %w", err)
	}
	notice.Signature = BytesToBase64(signature)
	return nil
}

// VerifyRecoveryChainsNotice checks notice's signature with dc, the handle
// of notice.CommitterDID.
func VerifyRecoveryChainsNotice(dc types.DIDCrypto, notice *types.RecoveryChainsNotice) error {
	if dc == nil || notice == nil || notice.CommitterDID != dc.GetDID() {
		return fmt.Errorf("recovery chains notice: committer DID does not match the DID handle")
	}
	signature, err := Base64ToBytes(notice.Signature)
	if err != nil || len(signature) == 0 {
		return fmt.Errorf("recovery chains notice: missing or invalid signature")
	}
	hash, err := recoveryChainsNoticeHash(*notice)
	if err != nil {
		return err
	}
	ok, err := dc.SignVerify(hash, signature)
	if err != nil {
		return fmt.Errorf("recovery chains notice: verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("recovery chains notice: signature verification failed")
	}
	return nil
}

// recoveryChainsNoticeHash hashes notice without its signature.
func recoveryChainsNoticeHash(notice types.RecoveryChainsNotice) ([]byte, error) {
	notice.Signature = ""
	data, err := json.Marshal(notice)
	if err != nil {
		return nil, fmt.Errorf("recovery chains notice: encode: %w", err)
	}
	return CalculateHash(data, constants.HashAlgorithm_SHA3_256), nil
}
