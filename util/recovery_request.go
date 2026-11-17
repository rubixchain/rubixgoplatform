package util

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
)

// Pass the DID handle from SetupDID to use the existing signing flow.
func SignRecoveryRequest(dc types.DIDCrypto, request *types.RecoveryRequest) error {
	if err := checkRecoveryRequest(dc, request); err != nil {
		return err
	}
	data, err := request.SigningBytes()
	if err != nil {
		return fmt.Errorf("recovery request: encode signing bytes: %w", err)
	}
	signature, err := dc.Sign(CalculateHash(data, constants.HashAlgorithm_SHA3_256))
	if err != nil {
		return fmt.Errorf("recovery request: sign: %w", err)
	}
	request.Signature = BytesToBase64(signature)
	return nil
}

// Pass the DID handle from InitialiseDID and this node's network.
func VerifyRecoveryRequest(dc types.DIDCrypto, request *types.RecoveryRequest, network string) error {
	if err := checkRecoveryRequest(dc, request); err != nil {
		return err
	}
	if request.Network != network {
		return fmt.Errorf("recovery request: network does not match this node")
	}
	if request.Signature == "" {
		return fmt.Errorf("recovery request: signature is required")
	}
	signature, err := Base64ToBytes(request.Signature)
	if err != nil {
		return fmt.Errorf("recovery request: decode signature: %w", err)
	}
	data, err := request.SigningBytes()
	if err != nil {
		return fmt.Errorf("recovery request: encode signing bytes: %w", err)
	}
	ok, err := dc.SignVerify(CalculateHash(data, constants.HashAlgorithm_SHA3_256), signature)
	if err != nil {
		return fmt.Errorf("recovery request: verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("recovery request: signature verification failed")
	}
	return nil
}

func checkRecoveryRequest(dc types.DIDCrypto, request *types.RecoveryRequest) error {
	if dc == nil || request == nil {
		return fmt.Errorf("recovery request: DID handle and request are required")
	}
	if request.DID == "" || request.DID != dc.GetDID() {
		return fmt.Errorf("recovery request: DID does not match the DID handle")
	}
	if request.Network == "" {
		return fmt.Errorf("recovery request: network is required")
	}
	if request.Purpose != types.RecoveryRequestPurpose {
		return fmt.Errorf("recovery request: purpose must be %q", types.RecoveryRequestPurpose)
	}
	if request.Epoch <= 0 {
		return fmt.Errorf("recovery request: epoch must be a positive Unix time in seconds")
	}
	return nil
}
