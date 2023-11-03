package main

import (
	"os"

	_ "github.com/gklps/ensweb"
	"github.com/rubixchain/rubixgoplatform/command"
)

// @title Rubix Core
// @version 0.9
// @description Rubix core API to control & manage the node.

// @contact.name API Support
// @contact.email murali.c@ensurity.com

// @BasePath

// @securityDefinitions.apikey SessionToken
// @in header
// @name Session-Token
func main() {
	command.Run(os.Args)
}
