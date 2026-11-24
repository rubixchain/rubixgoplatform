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

// @securityDefinitions.apikey NodeAPIKey
// @in header
// @name X-API-Key
// @description Node API key; view it on the node with `rubixgoplatform getapikey -p <nodeDir>`.
func main() {
	command.Run(os.Args)
}
