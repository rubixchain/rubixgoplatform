package command
import (
	"fmt"
	"regexp"
	"strings"
)
func (cmd *Command) MineRBTs() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get DID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	br, err := cmd.c.MineRBTs(cmd.did)
	if err != nil {
		cmd.log.Info("Cannot mine RBTs")
		return
	}
	fmt.Println(br.Message)
	cmd.log.Info("RBT's mined successfully for the given token credits.")
}