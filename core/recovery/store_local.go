package recovery

import (
	"context"
	"fmt"
)

// LocalTokenState is the subset of a local token row used to compare against the
// fullnode copy during delta and dry-run recovery.
type LocalTokenState struct {
	TokenStateHash string
	TokenStatus    int16
	LatestPosition int64
}

// GetLocalTokenState returns the current local state of every token owned by
// did, keyed by token id, for delta classification. It reuses Wallet.GetAllTokens
// so the read matches what a normal balance query sees.
func (s *Store) GetLocalTokenState(ctx context.Context, did string) (map[string]LocalTokenState, error) {
	if did == "" {
		return nil, fmt.Errorf("GetLocalTokenState: did is required")
	}
	tokens, err := s.w.GetAllTokens(did)
	if err != nil {
		return nil, fmt.Errorf("GetLocalTokenState: %w", err)
	}
	out := make(map[string]LocalTokenState, len(tokens))
	for i := range tokens {
		out[tokens[i].TokenID] = LocalTokenState{
			TokenStateHash: tokens[i].TokenStateHash,
			TokenStatus:    tokens[i].TokenStatus,
			LatestPosition: tokens[i].LatestPosition,
		}
	}
	return out, nil
}

// VerifyRecoveredToken checks that a recovered token was rebuilt coherently: its
// local tokens row carries the expected state hash and its chain has at least the
// expected number of rows. It reuses the wallet read path so the self-test sees
// exactly what a later spend would read.
func (s *Store) VerifyRecoveredToken(tokenID, expectStateHash string, expectChainLen int) error {
	if tokenID == "" {
		return fmt.Errorf("VerifyRecoveredToken: token id is required")
	}
	tok, err := s.w.GetTokenByTokenID(tokenID)
	if err != nil {
		return fmt.Errorf("VerifyRecoveredToken: read token %q: %w", tokenID, err)
	}
	if tok.TokenStateHash != expectStateHash {
		return fmt.Errorf("VerifyRecoveredToken: token %q state hash mismatch (local %q, want %q)", tokenID, tok.TokenStateHash, expectStateHash)
	}
	chain, err := s.w.GetTokenChainByTokenID(tokenID, false)
	if err != nil {
		return fmt.Errorf("VerifyRecoveredToken: read chain %q: %w", tokenID, err)
	}
	if len(chain) < expectChainLen {
		return fmt.Errorf("VerifyRecoveredToken: token %q chain length %d is less than expected %d", tokenID, len(chain), expectChainLen)
	}
	return nil
}
