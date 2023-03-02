package command

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/EnsurityTechnologies/apiconfig"
	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/server"
	"golang.org/x/term"
)

const (
	ConfigFile string = "api_config.json"
)

const (
	version string = "0.0.3"
)
const (
	VersionCmd            string = "-v"
	HelpCmd               string = "-h"
	RunCmd                string = "run"
	PingCmd               string = "ping"
	AddBootStrapCmd       string = "addbootstrap"
	RemoveBootStrapCmd    string = "removebootstrap"
	RemoveAllBootStrapCmd string = "removeallbootstrap"
	GetAllBootStrapCmd    string = "getallbootstrap"
	CreateDIDCmd          string = "createdid"
	GetAllDIDCmd          string = "getalldid"
	AddQuorumCmd          string = "addquorum"
	GetAllQuorumCmd       string = "getallquorum"
	RemoveAllQuorumCmd    string = "removeallquorum"
	SetupQuorumCmd        string = "setupquorum"
	GenerateTestRBTCmd    string = "generatetestrbt"
	TransferRBTCmd        string = "transferrbt"
	GetAccountInfoCmd     string = "getaccountinfo"
	SetupServiceCmd       string = "setupservice"
	DumpTokenChainCmd     string = "dumptokenchain"
	RegsiterDIDCmd        string = "registerdid"
	ShutDownCmd           string = "shutdown"
	MirgateNodeCmd        string = "migratenode"
	LockTokensCmd         string = "locktokens"
	CreateDataTokenCmd    string = "createdatatoken"
)

var commands = []string{VersionCmd,
	HelpCmd,
	RunCmd,
	PingCmd,
	AddBootStrapCmd,
	RemoveBootStrapCmd,
	RemoveAllBootStrapCmd,
	GetAllBootStrapCmd,
	CreateDIDCmd,
	GetAllDIDCmd,
	AddQuorumCmd,
	GetAllQuorumCmd,
	RemoveAllQuorumCmd,
	SetupQuorumCmd,
	GenerateTestRBTCmd,
	TransferRBTCmd,
	GetAccountInfoCmd,
	SetupServiceCmd,
	DumpTokenChainCmd,
	RegsiterDIDCmd,
	ShutDownCmd,
	MirgateNodeCmd,
	LockTokensCmd,
	CreateDataTokenCmd}
var commandsHelp = []string{"To get tool version",
	"To get help",
	"To run the rubix core",
	"This command will be used to ping the peer",
	"This command will add bootstrap peers to the configuration",
	"This command will remove bootstrap peers from the configuration",
	"This command will remove all bootstrap peers from the configuration",
	"This command will get all bootstrap peers from the configuration",
	"This command will create DID",
	"This command will get all DID address",
	"This command will add quorurm list to node",
	"This command will get all quorurm list from node",
	"This command will delete all quorurm list from node",
	"This command will setup node as quorurm",
	"This command will generate test RBT token",
	"This command will trasnfer RBT",
	"This command will help to get account information",
	"This command enable explorer service on the node",
	"This command will dump the token chain into file",
	"This command will register DID peer map across the network",
	"This command will shutdown the rubix node",
	"This command will migrate node to newer node",
	"This command will lock the tokens on the arbitary node",
	"This command will create data token token"}

type Command struct {
	cfg          config.Config
	c            *client.Client
	encKey       string
	start        bool
	node         uint
	runDir       string
	logFile      string
	logLevel     string
	cfgFile      string
	testNet      bool
	testNetKey   string
	addr         string
	port         string
	peerID       string
	peers        []string
	log          logger.Logger
	didType      int
	didSecret    string
	forcePWD     bool
	privPWD      string
	quorumPWD    string
	imgFile      string
	didImgFile   string
	privImgFile  string
	pubImgFile   string
	privKeyFile  string
	pubKeyFile   string
	quorumList   string
	srvName      string
	dbName       string
	dbType       string
	dbAddress    string
	dbPort       string
	dbUserName   string
	dbPassword   string
	senderAddr   string
	receiverAddr string
	rbtAmount    float64
	transComment string
	transType    int
	numTokens    int
	enableAuth   bool
	did          string
	token        string
	arbitaryMode bool
	tokenList    string
	fileMode     bool
	file         string
	userID       string
	userInfo     string
}

func showVersion() {
	fmt.Printf("\n****************************************\n\n")
	fmt.Printf("Rubix Core Version : %s\n", version)
	fmt.Printf("\n****************************************\n\n")
}

func showHelp() {
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

func (cmd *Command) runApp() {
	core.InitConfig(cmd.runDir+cmd.cfgFile, cmd.encKey, uint16(cmd.node))
	err := apiconfig.LoadAPIConfig(cmd.runDir+cmd.cfgFile, cmd.encKey, &cmd.cfg)
	if err != nil {
		cmd.log.Error("Configfile is either currupted or cipher is wrong", "err", err)
		return
	}

	// Override directory path
	cmd.cfg.DirPath = cmd.runDir
	sc := make(chan bool, 1)
	c, err := core.NewCore(&cmd.cfg, cmd.runDir+cmd.cfgFile, cmd.encKey, cmd.log, cmd.testNet, cmd.testNetKey, cmd.arbitaryMode)
	if err != nil {
		cmd.log.Error("failed to create core")
		return
	}
	scfg := &server.Config{
		Config: srvcfg.Config{
			HostAddress: cmd.cfg.NodeAddress,
			HostPort:    cmd.cfg.NodePort,
		},
	}
	scfg.EnableAuth = cmd.enableAuth
	if cmd.enableAuth {
		scfg.DBType = "Sqlite3"
		scfg.DBAddress = cmd.cfg.DirPath + "rubix.db"
	}
	// scfg := &srvcfg.Config{
	// 	HostAddress: cmd.cfg.NodeAddress,
	// 	HostPort:    cmd.cfg.NodePort,
	// }
	s, err := server.NewServer(c, scfg, cmd.log, cmd.start, sc)
	if err != nil {
		cmd.log.Error("Failed to create server")
		return
	}
	cmd.log.Info("Core version : " + version)
	cmd.log.Info("Starting server...")
	go s.Start()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	signal.Notify(ch, syscall.SIGINT)
	select {
	case <-ch:
	case <-sc:
	}
	s.Shutdown()
	cmd.log.Info("Shutting down...")
}

func (cmd *Command) validateOptions() bool {
	if cmd.runDir == "" {
		cmd.runDir = "./"
	}
	if !strings.HasPrefix(cmd.runDir, "\\") {
		if !strings.HasPrefix(cmd.runDir, "/") {
			cmd.runDir = cmd.runDir + "/"
		}
	}
	_, err := os.Stat(cmd.runDir)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.MkdirAll(cmd.runDir, os.ModeDir|os.ModePerm)
		if err == nil || os.IsExist(err) {
			return true
		} else {
			return false
		}
	}
	return false
}

func Run(args []string) {

	cmd := &Command{}
	var peers string

	flag.StringVar(&cmd.runDir, "p", "./", "Working directory path")
	flag.StringVar(&cmd.logFile, "logFile", "", "Log file name")
	flag.StringVar(&cmd.logLevel, "logLevel", "debug", "Log level")
	flag.StringVar(&cmd.cfgFile, "c", ConfigFile, "Configuration file for the core")
	flag.UintVar(&cmd.node, "n", 0, "Node number")
	flag.StringVar(&cmd.encKey, "k", "TestKeyBasic#2022", "Config file encryption key")
	flag.BoolVar(&cmd.start, "s", false, "Start the core")
	flag.BoolVar(&cmd.testNet, "testNet", false, "Run as test net")
	flag.StringVar(&cmd.testNetKey, "testNetKey", "testswarm.key", "Test net key")
	flag.StringVar(&cmd.addr, "addr", "localhost", "Server/Host Address")
	flag.StringVar(&cmd.port, "port", "20000", "Server/Host port")
	flag.StringVar(&cmd.peerID, "peerID", "", "Peerd ID")
	flag.StringVar(&peers, "peers", "", "Bootstrap peers, mutiple peers will be seprated by comma")
	flag.IntVar(&cmd.didType, "didType", 0, "DID Creation type")
	flag.StringVar(&cmd.didSecret, "didSecret", "My DID Secret", "DID creation secret")
	flag.BoolVar(&cmd.forcePWD, "fp", false, "Force password entry")
	flag.StringVar(&cmd.privPWD, "privPWD", "mypassword", "Private key password")
	flag.StringVar(&cmd.quorumPWD, "quorumPWD", "mypassword", "Quorum key password")
	flag.StringVar(&cmd.imgFile, "imgFile", did.ImgFileName, "DID creation image")
	flag.StringVar(&cmd.didImgFile, "didImgFile", did.DIDImgFileName, "DID image")
	flag.StringVar(&cmd.privImgFile, "privImgFile", did.PvtShareFileName, "DID public share image")
	flag.StringVar(&cmd.pubImgFile, "pubImgFile", did.PubShareFileName, "DID public share image")
	flag.StringVar(&cmd.privKeyFile, "privKeyFile", did.PvtKeyFileName, "Private key file")
	flag.StringVar(&cmd.pubKeyFile, "pubKeyFile", did.PubKeyFileName, "Public key file")
	flag.StringVar(&cmd.quorumList, "quorumList", "quorumlist.json", "Quorum list")
	flag.StringVar(&cmd.srvName, "srvName", "explorer_service", "Service name")
	flag.StringVar(&cmd.dbName, "dbName", "ServiceDB", "Service database name")
	flag.StringVar(&cmd.dbType, "dbType", "SQLServer", "DB Type, supported database are SQLServer, PostgressSQL, MySQL & Sqlite3")
	flag.StringVar(&cmd.dbAddress, "dbAddress", "localhost", "Database address")
	flag.StringVar(&cmd.dbPort, "dbPort", "1433", "Database port number")
	flag.StringVar(&cmd.dbUserName, "dbUsername", "sa", "Database username")
	flag.StringVar(&cmd.dbPassword, "dbPassword", "password", "Database password")
	flag.StringVar(&cmd.senderAddr, "senderAddr", "", "Sender address")
	flag.StringVar(&cmd.receiverAddr, "receiverAddr", "", "Receiver address")
	flag.Float64Var(&cmd.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	flag.StringVar(&cmd.transComment, "transComment", "Test tranasaction", "Transaction comment")
	flag.IntVar(&cmd.transType, "transType", 2, "Transaction type")
	flag.IntVar(&cmd.numTokens, "numTokens", 1, "Number of tokens")
	flag.StringVar(&cmd.did, "did", "", "DID")
	flag.BoolVar(&cmd.enableAuth, "enableAuth", false, "Enable authentication")
	flag.BoolVar(&cmd.arbitaryMode, "arbitaryMode", false, "Enable arbitary mode")
	flag.StringVar(&cmd.tokenList, "tokenList", "tokens.txt", "Token lis")
	flag.StringVar(&cmd.token, "token", "", "Token name")
	flag.BoolVar(&cmd.fileMode, "fmode", false, "File mode")
	flag.StringVar(&cmd.file, "file", "file.txt", "File to be uploaded")
	flag.StringVar(&cmd.userID, "uid", "testuser", "User ID for token creation")
	flag.StringVar(&cmd.userInfo, "uinfo", "", "User info for token creation")

	if len(os.Args) < 2 {
		fmt.Println("Invalid Command")
		showHelp()
		return
	}

	cmdName := os.Args[1]

	os.Args = os.Args[1:]

	flag.Parse()

	if peers != "" {
		peers = strings.ReplaceAll(peers, " ", "")
		cmd.peers = strings.Split(peers, ",")
	}

	if !cmd.validateOptions() {
		fmt.Println("Validate options failed")
		return
	}

	if cmd.logFile == "" {
		cmd.logFile = cmd.runDir + "log.txt"
	}

	level := logger.Debug

	fp, err := os.OpenFile(cmd.logFile,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	switch strings.ToLower(cmd.logLevel) {
	case "error":
		level = logger.Error
	case "info":
		level = logger.Info
	case "debug":
		level = logger.Debug
	default:
		level = logger.Debug
	}

	logOptions := &logger.LoggerOptions{
		Name:   "Main",
		Level:  level,
		Color:  []logger.ColorOption{logger.AutoColor, logger.ColorOff},
		Output: []io.Writer{logger.DefaultOutput, fp},
	}

	cmd.log = logger.New(logOptions)

	cmd.c, err = client.NewClient(&srvcfg.Config{ServerAddress: cmd.addr, ServerPort: cmd.port}, cmd.log)
	if err != nil {
		cmd.log.Error("Failed to create client")
		return
	}

	switch cmdName {
	case VersionCmd:
		showVersion()
	case HelpCmd:
		showHelp()
	case RunCmd:
		cmd.runApp()
	case PingCmd:
		cmd.ping()
	case AddBootStrapCmd:
		cmd.addBootStrap()
	case RemoveBootStrapCmd:
		cmd.removeBootStrap()
	case RemoveAllBootStrapCmd:
		cmd.removeAllBootStrap()
	case GetAllBootStrapCmd:
		cmd.getAllBootStrap()
	case CreateDIDCmd:
		cmd.CreateDID()
	case GetAllDIDCmd:
		cmd.GetAllDID()
	case AddQuorumCmd:
		cmd.AddQuorurm()
	case GetAllQuorumCmd:
		cmd.GetAllQuorum()
	case RemoveAllQuorumCmd:
		cmd.RemoveAllQuorum()
	case SetupQuorumCmd:
		cmd.SetupQuorum()
	case SetupServiceCmd:
		cmd.SetupService()
	case GenerateTestRBTCmd:
		cmd.GenerateTestRBT()
	case TransferRBTCmd:
		cmd.TransferRBT()
	case GetAccountInfoCmd:
		cmd.GetAccountInfo()
	case DumpTokenChainCmd:
		cmd.dumpTokenChain()
	case RegsiterDIDCmd:
		cmd.RegsiterDIDCmd()
	case ShutDownCmd:
		cmd.ShutDownCmd()
	case MirgateNodeCmd:
		cmd.MigrateNodeCmd()
	case CreateDataTokenCmd:
		cmd.createDataToken()
	default:
		cmd.log.Error("Invalid command")
	}
}

func (cmd *Command) basicClient(method string, path string, model interface{}) (ensweb.Client, *http.Request, error) {
	cfg := srvcfg.Config{
		ServerAddress: cmd.addr,
		ServerPort:    cmd.port,
	}
	c, err := ensweb.NewClient(&cfg, cmd.log)
	if err != nil {
		return c, nil, fmt.Errorf("failed to get new client, " + err.Error())
	}
	r, err := c.JSONRequest(method, path, model)
	if err != nil {
		return c, nil, fmt.Errorf("failed to create http request, " + err.Error())
	}
	return c, r, nil
}

func (cmd *Command) multiformClient(method string, path string, field map[string]string, files map[string]string) (ensweb.Client, *http.Request, error) {
	cfg := srvcfg.Config{
		ServerAddress: cmd.addr,
		ServerPort:    cmd.port,
	}
	c, err := ensweb.NewClient(&cfg, cmd.log)
	if err != nil {
		return c, nil, fmt.Errorf("failed to get new client, " + err.Error())
	}
	r, err := c.MultiFormRequest(method, path, field, files)
	if err != nil {
		return c, nil, fmt.Errorf("failed to create http request, " + err.Error())
	}
	return c, r, nil
}

func getpassword(msg string) (string, error) {
	fmt.Print(msg)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}
