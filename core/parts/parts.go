package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/core/coin"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func floatPrecision(num float64, precision int) float64 {
	precision = coin.MaxSupportedDecimalPlaces
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func GetRequiredAmountDenom(requiredAmount float64) ([]string, error) {
	tokenDenomArr := wallet.CreateTokenDenomArr(coin.MaxSupportedDecimalPlaces)
	maxLevel := wallet.GetMaxLevel(coin.MaxSupportedDecimalPlaces)
	remaining := requiredAmount

	for level := 0; level < maxLevel; level++ {
		unit, err := wallet.IdxToDenom(level) // 1, 0.01, 0.05
		if err != nil {
			return nil, fmt.Errorf("GetRequiredAmountDenom: failed to get the denom for idx: %v, err: %v", level, err)
		}

		count := int(remaining / unit)
		tokenDenomArr[level] = floatToInt(count)
		remaining -= float64(count) * unit
		remaining = floatPrecision(remaining, 3)
	}

	if remaining != floatPrecision(0, 3) {
		return nil, fmt.Errorf("target amount not representable with current precision, remaining: %v", remaining)
	}

	return tokenDenomArr, nil
}

func TokenSplit(dc did.DIDCrypto, w *wallet.Wallet, did string, balanceDenom []string, targetTokenDenom []string, ipfsOps IPFSOperation, isTestnet bool, log logger.Logger) ([]*wallet.Token, error) {
	// maxLevel = MaxLevel() + 1
	operations := []Operation{}

	maxLevel := wallet.GetMaxLevel(coin.MaxSupportedDecimalPlaces)
	intermediateTokenDenomArr := wallet.CreateTokenDenomArr(coin.MaxSupportedDecimalPlaces)
	maxLevelBoundary := maxLevel - 1

	var requiredTokens []*wallet.Token = make([]*wallet.Token, 0)

	for level := maxLevelBoundary; level >= 0; level-- {
		requiredDenomCountAtLevel, err := wallet.GetTokenDenomCount(targetTokenDenom, level)
		if err != nil {
			
			return nil, fmt.Errorf("TokenSplit: error occured while fetching token denom count for level: %v, err: %v", level, err)
		}

		if requiredDenomCountAtLevel == 0 {
			continue
		}

		{
			// TO BE REMOVED
			denomATIDX, err := wallet.IdxToDenom(level)
			if err != nil {
				return nil, fmt.Errorf("LOG 1: unexpected error: %v", err)
			}
			
			availableDenomCountAtLevel, err := wallet.GetTokenDenomCount(balanceDenom, level)
			if err != nil {
				return nil, fmt.Errorf("LOG 2: expected err: %v", err)
			}
			log.Warn(fmt.Sprintf("\n[Level %d (%v)] Need: %d, Have: %d\n", 
				level, denomATIDX, requiredDenomCountAtLevel, availableDenomCountAtLevel))
		}

		for requiredDenomCountAtLevel > 0 {
			availableDenomCountAtLevel, err := wallet.GetTokenDenomCount(balanceDenom, level)
			if err != nil {
				
				return nil, fmt.Errorf("TokenSplit: error occured while fetching available token denom at level: %v, err: %v", level, err)
			}

			if availableDenomCountAtLevel >= requiredDenomCountAtLevel {
				errDecrement := wallet.DecrementTokenDenomArrayAtIndex(balanceDenom, level, requiredDenomCountAtLevel)
				if errDecrement != nil {
					
					return nil, fmt.Errorf("TokenSplit: error occurred while decrementing user's balance denom array, err: %v", errDecrement)
				}

				errIncrement := wallet.IncrementTokenDenomArrayAtIndex(intermediateTokenDenomArr, level, requiredDenomCountAtLevel)
				if errIncrement != nil {
					
					return nil, fmt.Errorf("TokenSplit: error occurred while incrementing intermediate denom array, err: %v", errIncrement)
				}

				denomAtLevel, err := wallet.IdxToDenom(level)
				if err != nil {
					
					return nil, err
				}

				for i := 1; i <= requiredDenomCountAtLevel; i++ {
					token, err := w.GetTokenByValueAndStatus(did, denomAtLevel, wallet.TokenIsFree)
					if err != nil {
						
						return nil, fmt.Errorf("splitAtLevel (initial): error occurred while looking for an available parent token in DB for denom:%v, err: %v", denomAtLevel, err)
					}

					log.Debug(fmt.Sprintf("STEP 1: TOKEN BEING CHOSEN: %v", token.TokenID))

					requiredTokens = append(requiredTokens, token)
				}

				op := Operation{
					Type:        "TAKE",
					Level:       level,
					Count:       requiredDenomCountAtLevel,
					Description: fmt.Sprintf("Take %v × %v", requiredDenomCountAtLevel, denomAtLevel),
				}
				operations = append(operations, op)

				requiredDenomCountAtLevel = 0
				break
			}

			if availableDenomCountAtLevel > 0 {
				errDecrement := wallet.DecrementTokenDenomArrayAtIndex(balanceDenom, level, availableDenomCountAtLevel)
				if errDecrement != nil {
					
					return nil, fmt.Errorf("TokenSplit: error occurred while decrementing user's balance denom array, err: %v", errDecrement)
				}

				errIncrement := wallet.IncrementTokenDenomArrayAtIndex(intermediateTokenDenomArr, level, availableDenomCountAtLevel)
				if errIncrement != nil {
					
					return nil, fmt.Errorf("TokenSplit: error occurred while incrementing intermediate denom array, err: %v", errIncrement)
				}

				denomAtLevel, err := wallet.IdxToDenom(level)
				if err != nil {
					
					return nil, err
				}

				for i := 1; i <= availableDenomCountAtLevel; i++ {
					token, err := w.GetTokenByValueAndStatus(did, denomAtLevel, wallet.TokenIsFree)
					if err != nil {
						
						return nil, fmt.Errorf("TokenSplit: error occurred while looking for an available token in DB for denom:%v, err: %v", denomAtLevel, err)
					}

					log.Debug(fmt.Sprintf("STEP 2: TOKEN BEING CHOSEN: %v", token.TokenID))
					requiredTokens = append(requiredTokens, token)
				}

				op := Operation{
					Type:        "TAKE",
					Level:       level,
					Count:       availableDenomCountAtLevel,
					Description: fmt.Sprintf("Take %v × %v (partial)", availableDenomCountAtLevel, denomAtLevel),
				}
				operations = append(operations, op)

				requiredDenomCountAtLevel -= availableDenomCountAtLevel
			}

			// RADIUS SEARCH
			denomValueAtLevel, err := wallet.IdxToDenom(level)
			if err != nil {
				
				return nil, fmt.Errorf("TokenSplit: error occured while fetching denom value at index: %v, err: %v", level, err)
			}

			requiredValue := floatMultiply(denomValueAtLevel, requiredDenomCountAtLevel)
			if requiredValue == 0.0 {
				
				return nil, fmt.Errorf("TokenSplit: invalid required value needed for spliting, value: %v", requiredValue)
			}
			log.Warn(fmt.Sprintf("  → Starting radius search for value: %v\n", requiredValue))


			found := false
			for distance := 1; distance <= maxLevelBoundary && !found; distance++ {
				log.Warn(fmt.Sprintf("  → Radius %v:\n", distance))
				// Right Aggregation
				right := level + distance
				if right <= maxLevelBoundary {
					denomAtRight, _ := wallet.IdxToDenom(right)

					log.Warn(fmt.Sprintf("    • Checking RIGHT: level %v (%v)\n", 
						right, denomAtRight))

					constructed := float64(0)
					takenMap := make(map[int]float64)

					for currentLevel := right; currentLevel <= maxLevelBoundary && constructed < requiredValue; currentLevel++ {
						rightDenomValue, err := wallet.IdxToDenom(currentLevel)
						if err != nil {
							
							return nil, err
						}

						availableRightDenomCount, err := wallet.GetTokenDenomCount(balanceDenom, currentLevel)
						if err != nil {
							
							return nil, err
						}

						canTake := getMinimum(availableRightDenomCount, int((requiredValue-constructed)/rightDenomValue))

						if canTake > 0 {
							takenMap[currentLevel] = float64(canTake)
							constructed += float64(canTake) * rightDenomValue

							log.Warn(fmt.Sprintf("      - Can use %v × level %d (%s) | constructed=%v\n",
								canTake, currentLevel, rightDenomValue, 
								constructed))
						}
					}

					if constructed == requiredValue {
						log.Warn(fmt.Sprintf("    ✅ AGGREGATE: Exact match found at radius %v!\n", distance))
						for level, count := range takenMap {
							errDecrement := wallet.DecrementTokenDenomArrayAtIndex(balanceDenom, level, int(count))
							if errDecrement != nil {
								
								return nil, fmt.Errorf("TokenSplit: error occurred while decrementing user's balance denom array, err: %v", errDecrement)
							}

							errIncrement := wallet.IncrementTokenDenomArrayAtIndex(intermediateTokenDenomArr, level, int(count))
							if errIncrement != nil {
								
								return nil, fmt.Errorf("TokenSplit: error occurred while incrementing intermediate denom array, err: %v", errIncrement)
							}

							denomAtLevel, err := wallet.IdxToDenom(level)
							if err != nil {
								
								return nil, err
							}

							for i := 1; i <= int(count); i++ {
								token, err := w.GetTokenByValue(did, denomAtLevel)
								if err != nil {
									
									return nil, fmt.Errorf("splitAtLevel(final): error occurred while looking for an available parent token in DB for denom:%v, err: %v", denomAtLevel, err)
								}

								log.Debug(fmt.Sprintf("STEP: 3: TOKEN BEING CHOSEN: %v", token.TokenID))

								requiredTokens = append(requiredTokens, token)
							}

							op := Operation{
								Type:        "AGGREGATE",
								Level:       level,
								Count:       int(count),
								Description: fmt.Sprintf("Aggregate %v × %v (for level %v, radius %v)", 
									int(count), denomAtLevel, level, distance),
							}
							operations = append(operations, op)
						}

						requiredDenomCountAtLevel = 0
						found = true
						break
					} else if constructed > 0 {
						fmt.Printf("      ✗ Constructed %v ≠ %v (not exact)\n", 
							constructed, 
							requiredValue)
					}
				}

				// Left Aggregation
				if !found {
					left := level - distance
					if left >= 0 {
						balanceDenomCountAtLevel, err := wallet.GetTokenDenomCount(balanceDenom, left)
						if err != nil {
							
							return nil, err
						}

						log.Warn(fmt.Sprintf("    • Checking LEFT: level %d (%v)\n", 
							left, balanceDenomCountAtLevel))

						if balanceDenomCountAtLevel > 0 {
							log.Warn(fmt.Sprintf("    ✅ SPLIT: Found token at level %v (radius %v)\n", 
								left, distance))
							log.Warn(fmt.Sprintf("      ⚠ SPLIT: level %d (%s)\n", 
								left, balanceDenomCountAtLevel))
							
							requiredTokens, err = splitAtLevel(dc, w, did, requiredTokens, balanceDenom, left, ipfsOps, isTestnet)
							if err != nil {
								
								return nil, err
							}

							fmt.Printf("Step (n+1): Inside splitAtLevel: token denom arr: %v", balanceDenom)
							denomAtCurrentLevel, _ := wallet.IdxToDenom(left)
							denomAtNextLevel, _ := wallet.IdxToDenom(left+1)
							op := Operation{
								Type:        "SPLIT",
								Level:       left,
								Count:       1,
								Description: fmt.Sprintf("Split 1 × %s → %d × %s (radius %d)", 
									denomAtCurrentLevel, 
									SplitFactor(left+1), 
									denomAtNextLevel,
									distance),
							}
							operations = append(operations, op)
							
							found = true
							break
						} else {
							log.Warn(fmt.Sprintf("      ✗ No tokens at level %v\n", left))
						}
					}
				}
			}

			if !found {
				
				return nil, fmt.Errorf("TokenSplit: unexpected error, unable to fulfil requirement at level: %v due to insufficient balance", level)
			}
		}
	}

	
	_ = TransferResult{
		Transferred: intermediateTokenDenomArr,
		Operations:  operations,
	}


	log.Warn(fmt.Sprintf("Token Denom right after: %v", balanceDenom))

	return requiredTokens, nil
}
