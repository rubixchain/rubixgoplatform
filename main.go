package main

import (
	"github.com/rubixchain/rubixgoplatform/command"
	_ "github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func main() {
	// Rubix CLI Entrypoint
	command.Execute()
}
