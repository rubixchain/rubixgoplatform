package main

import (
	"fmt"
	"log"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func setupDev() (*core.Core, types.RubixConfig) {
	userCfg, err := config.ParseConfigFromPath("./config")
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.CreateRubixConfigFromUserConfig(userCfg, ".")
	if err != nil {
		log.Fatal(err)
	}

	cfg.CfgData.Ports = cfg.PortConfig

	lg := logger.New(&logger.LoggerOptions{Name: "dev-v2"})

	c, err := core.NewCore(&cfg, lg, "localnet", false, "")
	if err != nil {
		log.Fatal(err)
	}

	if err := c.RunIPFS(); err != nil {
		log.Fatal(err)
	}

	c.InitDIDModule()

	return c, cfg
}

func createDIDs(c *core.Core, count int) []string {
	dids := make([]string, 0, count)
	for range count {
		didID, err := c.CreateDID(&types.DIDCreate{
			PrivPWD: "pwd-1",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Created DID:", didID)
		dids = append(dids, didID)
	}
	return dids
}
