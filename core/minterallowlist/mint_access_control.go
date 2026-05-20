// Package minterallowlist holds the list of DIDs allowed to mint tokens, used
// to check transfers.
package minterallowlist

// MintAccessRange is one minter's allowed token-number range at a given level.
type MintAccessRange struct {
	DID              string
	Level            int
	StartTokenNumber int
	EndTokenNumber   int
}

// AllowedMinters lists the DIDs that minted mainnet tokens, and which token
// numbers each one owns.
var AllowedMinters = []MintAccessRange{
	{DID: "bafybmid35wgktknwpaddfxkgdrouq5z2cmcdz27gf2u4xvrh4pswmrnvpe", Level: 1, StartTokenNumber: 1, EndTokenNumber: 400000},
	{DID: "bafybmifogacbfyny7wahapjuk7jpjl3ffylsakmtbrcqmdwckofnsg6rbu", Level: 1, StartTokenNumber: 400001, EndTokenNumber: 800000},
	{DID: "bafybmielnukpgfknvyrsocrvxshplwecjv3bqapt6my3juy4fa6gez4zmy", Level: 1, StartTokenNumber: 800001, EndTokenNumber: 1200000},
	{DID: "bafybmie6vd43nyvzesubkfddpydja6rwlxijsgkcjfsthovlbujclm3td4", Level: 1, StartTokenNumber: 1200001, EndTokenNumber: 1600000},
	{DID: "bafybmieybmtvh22j2kzjr5w4yzptbeax3musw7rcqwz6vkaejemn53qsvi", Level: 1, StartTokenNumber: 1600001, EndTokenNumber: 2000000},
	{DID: "bafybmighj3g2ip7c5wmfzulcfkdo6t4734oekbpeb7mh73nyp4sv4mvyqu", Level: 1, StartTokenNumber: 2000001, EndTokenNumber: 2400000},
	{DID: "bafybmifty4w2ccl3zcclaehgong5vhuocr3pnabccfnn27z2e6knxx7aye", Level: 1, StartTokenNumber: 2400001, EndTokenNumber: 2800000},
	{DID: "bafybmifk4dzbmjl434kdn32u4m4rzr6qgyvu7t4jtql2muqn7lx5fyj6aa", Level: 1, StartTokenNumber: 2800001, EndTokenNumber: 3200000},
	{DID: "bafybmihm6nutigowauas3232pgxhm3sk4xr4o7t7q7j4ejklh4hkmrrjna", Level: 1, StartTokenNumber: 3200001, EndTokenNumber: 3600000},
	{DID: "bafybmibw2b7hokbfvcdqb6u53gbve276oc2x2svaalklgzv4mzgfadw7cm", Level: 1, StartTokenNumber: 3600001, EndTokenNumber: 4000000},
	{DID: "bafybmifmwqhlscye636ui5ajybavpvyfb6gf25m3kkxhm7pewc6slh3yue", Level: 1, StartTokenNumber: 4000001, EndTokenNumber: 4300000},
}

// TestnetAllowedMinters lists the testnet faucet DID and its token-number
// range.
var TestnetAllowedMinters = []MintAccessRange{
	{DID: "bafybmiairxfiplfpwvzzgslubbx3dwqrutwhrgtypnzyb752rrjbthgrwm", Level: 50001, StartTokenNumber: 1, EndTokenNumber: 4300000},
}

// ValidateMinterAuthorization returns true if (did, level, number) is in the
// list.
func ValidateMinterAuthorization(table []MintAccessRange, did string, level, number int) bool {
	for _, m := range table {
		if m.DID == did && m.Level == level {
			return number >= m.StartTokenNumber && number <= m.EndTokenNumber
		}
	}
	return false
}

