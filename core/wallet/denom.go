package wallet

import (
	"fmt"

	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
)

func (w *Wallet) GetTokenDenomArray(did string) (map[types.DenomValue]types.DenomCount, error) {
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT denom, count FROM token_denom
		WHERE did=$1
	`, did)
	if err != nil {
		return nil, fmt.Errorf("GetTokenDenomArray: failed to query token denom array")
	}

	var tokenDenomMap map[types.DenomValue]types.DenomCount = make(map[types.DenomValue]types.DenomCount)
	for rows.Next() {
		var tokenDenomValue float64
		var tokenDenomCount int64
		err := rows.Scan(
			&tokenDenomValue, &tokenDenomCount,
		)
		if err != nil {
			return nil, err
		}

		tokenDenomMap[tokenDenomValue] = tokenDenomCount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetTokenDenomArray: failed while scanning rows: %v", err)
	}

	return tokenDenomMap, nil
}

func (w *Wallet) UpdateTokenDenomArray(did string, denom types.DenomValue, count types.DenomCount) error
func (w *Wallet) UpdateAllTokenDenomArray(did string, denomMap map[types.DenomValue]types.DenomCount) error

// The following function should only used when a new DID is created
func (w *Wallet) InitNewTokenDenomArrayForDID(did string, denomMap map[types.DenomValue]types.DenomCount) error
func (w *Wallet) GetRBTBalanceFromDenomArr(did string) (float64, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT denom, count
		FROM token_denom
		WHERE did=$1
		`, did,
	)
	if err != nil {
		return 0.0, err
	}

	var totalBalance float64 = rubixmath.ZeroFloat()
	for rows.Next() {
		var denomValue types.DenomValue
		var denomCount types.DenomCount

		err := rows.Scan(
			&denomValue, &denomCount,
		)
		if err != nil {
			return 0.0, err
		}

		totalDenomValue, err := rubixmath.ScaledMultFloatInt(denomValue, denomCount)
		if err != nil {
			return 0.0, fmt.Errorf("unexpected error occured while multiplying denom %v with count %v, err: %v", denomValue, denomCount, err)
		}

		totalBalance = rubixmath.AddFloat(totalBalance, totalDenomValue)
	}

	if err := rows.Err(); err != nil {
		return 0.0, err
	}

	return totalBalance, nil
}
