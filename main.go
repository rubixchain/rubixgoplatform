package main

import (
	"os"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/command"
)

func main() {

	// fp, err := os.OpenFile(cfg.LogFile,
	// 	os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	panic(err)
	// }

	logOptions := &logger.LoggerOptions{
		Name:  "Main",
		Level: logger.Debug,
		Color: logger.AutoColor,
		//	Output: fp,
	}

	log := logger.New(logOptions)

	command.Run(os.Args, log)
}
