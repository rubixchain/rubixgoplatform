package core
import(
	"fmt"
)

func (c *Core)MineRBTs(did string)error{
	fmt.Println("testing comment in core MineRBTs function")
	//1.check how many credits are there for mining at minor side
		// Fetch token details by QuorumDID, tokencreditStatus- 1(ready to mine)
		tokenDetails, err := c.w.GetTokenDetailsByQuorumDID(did,1)
		if err != nil {
			c.log.Error("Failed to fetch token details", "err", err)
			return nil // Return nil if fetching fails
		}
	// 2. Calculate total token credit
	totalCredits := 0
	for _, tokenInfos := range tokenDetails { // Iterate over the map values (slices of TokenInfo)
		for _, tokenInfo := range tokenInfos { // Iterate over each TokenInfo in the slice
			totalCredits += tokenInfo.TokenCredit // Sum up TokenCredit values
		}
	}
	//3.check how many are needed for mining next token from the mining chain
	//fetch credits required to mine next token from the mining chain
	creditsForNextToken:= 100

		// Check if total credits are sufficient
		if totalCredits < creditsForNextToken {
			errMsg := fmt.Sprintf("total credits (%d) are less than the required credits (%d) to mine the next token", totalCredits, creditsForNextToken)
			c.log.Error(errMsg)
			return fmt.Errorf(errMsg) // Return an error
		}
		//fetch mining quorum details

	//3.take all those token details and send it to mining quorum(to do that we might need an internal API)
	//4.Refer Initiate RBT flow for collecting and sending the token details.

return nil
}