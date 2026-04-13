package main

import (
	"os"

	"github.com/rubixchain/rubixgoplatform/command"
	_ "github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @title Rubix Core
// @version 1.0
// @description Rubix core API to control & manage the node.

// @BasePath

// @securityDefinitions.apikey SessionToken
// @in header
// @name Session-Token
func main() {
	command.Run(os.Args)
}
