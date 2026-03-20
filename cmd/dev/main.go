package main

import (
	"fmt"
	"log"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func main() {
	// Step 1: Parse config from ./config directory
	userCfg, err := config.ParseConfigFromPath("./config")
	if err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	// Step 2: Create RubixConfig from UserConfig, using "." as nodeDir
	cfg, err := config.CreateRubixConfigFromUserConfig(userCfg, ".")
	if err != nil {
		log.Fatalf("failed to create rubix config: %v", err)
	}

	// Step 3: Create logger
	lg := logger.New(&logger.LoggerOptions{Name: "dev"})

	// Step 4: Call NewCore -- note &cfg (address-of value)
	c, err := core.NewCore(&cfg, lg, "localnet", false, false, false, "")
	if err != nil {
		log.Fatalf("failed to create core: %v", err)
	}

	// Step 5: Print success
	fmt.Printf("Core initialized successfully: %+v\n", c)

	// Block forever
	select {}
}
