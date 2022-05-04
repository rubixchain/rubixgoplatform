package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/EnsurityTechnologies/apiconfig"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/server"
)

const (
	ConfigFile string = "api_config.json"
)

const (
	version string = "0.0.1"
)
const (
	VersionCmd string = "-v"
	HelpCmd    string = "-h"
	RunCmd     string = "run"
)

var commands = []string{VersionCmd,
	HelpCmd,
	RunCmd}
var commandsHelp = []string{"To get tool version",
	"To get help",
	"To run the rubix core"}

type MainHandle struct {
	cfg     config.Config
	encKey  string
	node    uint
	runDir  string
	cfgFile string
	log     logger.Logger
}

func ShowVersion() {
	fmt.Printf("\n****************************************\n\n")
	fmt.Printf("Rubix Core Version : %s\n", version)
	fmt.Printf("\n****************************************\n\n")
}

func ShowHelp() {
	if runtime.GOOS == "windows" {
		fmt.Printf("\nrubixgpplatform.exe <cmd>\n")
	} else {
		fmt.Printf("\nrubixgoplatform <cmd>\n")
	}
	fmt.Printf("\nUse the following commands\n\n")
	for i := range commands {
		fmt.Printf("     %20s : %s\n\n", commands[i], commandsHelp[i])
	}
}

func RunApp(h *MainHandle) {
	core.InitConfig(h.runDir+h.cfgFile, h.encKey, uint16(h.node))
	err := apiconfig.LoadAPIConfig(h.runDir+h.cfgFile, h.encKey, &h.cfg)
	if err != nil {
		h.log.Error("Configfile is either currupted or cipher is wrong", "err", err)
		return
	}

	// Override directory path
	h.cfg.DirPath = h.runDir

	s, err := server.NewServer(&h.cfg, h.log)
	if err != nil {
		h.log.Error("Failed to create server")
		return
	}
	h.log.Info("Starting server...")
	go s.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGINT)

	<-c
	s.Shutdown()
	h.log.Info("Shutting down...")
}

func (h *MainHandle) ValidateOptions() bool {
	if h.runDir == "" {
		h.runDir = "./"
	}
	if !strings.HasPrefix(h.runDir, "\\") {
		if !strings.HasPrefix(h.runDir, "/") {
			h.runDir = h.runDir + "/"
		}
	}
	_, err := os.Stat(h.runDir)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.MkdirAll(h.runDir, os.ModeDir|os.ModePerm)
		if err == nil || os.IsExist(err) {
			return true
		} else {
			return false
		}
	}
	return false
}

func main() {

	h := &MainHandle{}

	flag.StringVar(&h.runDir, "p", "./", "Working directory path")
	flag.StringVar(&h.cfgFile, "c", ConfigFile, "Configuration file for the core")
	flag.UintVar(&h.node, "n", 0, "Node number")
	flag.StringVar(&h.encKey, "k", "TestKeyBasic#2022", "Config file encryption key")

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

	h.log = logger.New(logOptions)

	if len(os.Args) < 2 {
		h.log.Error("Invalid Command")
		ShowHelp()
		return
	}

	cmd := os.Args[1]

	os.Args = os.Args[1:]

	flag.Parse()

	if !h.ValidateOptions() {
		h.log.Error("Validate options failed")
		return
	}

	switch cmd {
	case VersionCmd:
		ShowVersion()
	case HelpCmd:
		ShowHelp()
	case RunCmd:
		RunApp(h)
	default:
		h.log.Error("Invalid command")
	}
}
