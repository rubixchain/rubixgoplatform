package command

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	_ "github.com/rubixchain/rubixgoplatform/docs"
	"github.com/rubixchain/rubixgoplatform/server"
	"github.com/rubixchain/rubixgoplatform/types"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"golang.org/x/term"
)

const (
	version string = "0.2"
)

var (
	currentCommit  string = "unknown"
	previousCommit string = "unknown"
)

const (
	VersionCmd                 string = "-v"
	HelpCmd                    string = "-h"
	RunCmd                     string = "run"
	PingCmd                    string = "ping"
	AddBootStrapCmd            string = "addbootstrap"
	RemoveBootStrapCmd         string = "removebootstrap"
	RemoveAllBootStrapCmd      string = "removeallbootstrap"
	GetAllBootStrapCmd         string = "getallbootstrap"
	CreateDIDCmd               string = "createdid"
	GetAllDIDCmd               string = "getalldid"
	AddQuorumCmd               string = "addquorum"
	GetAllQuorumCmd            string = "getallquorum"
	RemoveAllQuorumCmd         string = "removeallquorum"
	SetupQuorumCmd             string = "setupquorum"
	GenerateLocalRBTCmd        string = "generatelocalrbt"
	GenerateMainnetRBTCmd      string = "generatemainnetrbt"
	RegsiterDIDCmd             string = "registerdid"
	SetupDIDCmd                string = "setupdid"
	ShutDownCmd                string = "shutdown"
	CreateNFTCmd               string = "create-nft"
	GenerateSmartContractToken string = "generatesct"
	FetchSmartContract         string = "fetchsct"
	SubscribeContractCmd       string = "subscribesct"
	GetPeerID                  string = "get-peer-id"
	CheckQuorumStatusCmd       string = "checkQuorumStatus"
	AddPeerDetailsCmd          string = "addpeerdetails"
	GenerateFaucetTestRBTCmd   string = "generatefaucetrbt"
	CreateFTCmd                string = "createft"
	SubscribeNFTCmd            string = "subscribe-nft"
	FetchNftCmd                string = "fetch-nft"
	AddPeerDetailsFromExplorer string = "exppeerdetails"
	ArbitrarySignCmd           string = "sign"
	VerifySignatureCmd         string = "verify-signature"
	RemoveStaleDIDCmd          string = "removedid"
	InitCmd                    string = "init"

	// balance commands
	GetDIDBalanceCmd string = "getdidbalance"
	GetFTBalanceCmd  string = "getftbalance"
	GetRBTBalanceCmd string = "getrbtbalance"
)

var commands = []string{
	VersionCmd,
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
	GenerateLocalRBTCmd,
	GenerateMainnetRBTCmd,
	GetRBTBalanceCmd,
	RegsiterDIDCmd,
	SetupDIDCmd,
	ShutDownCmd,
	SubscribeContractCmd,
	CreateNFTCmd,
	GenerateSmartContractToken,
	FetchSmartContract,
	GetPeerID,
	AddPeerDetailsCmd,
	CheckQuorumStatusCmd,
	GenerateFaucetTestRBTCmd,
	CreateFTCmd,
	GetFTBalanceCmd,
	SubscribeNFTCmd,
	FetchNftCmd,
	AddPeerDetailsFromExplorer,
	ArbitrarySignCmd,
	VerifySignatureCmd,
	RemoveStaleDIDCmd,
	GetDIDBalanceCmd,
	InitCmd,
}

var commandsHelp = []string{
	"To get tool version",
	"To get help",
	"To run the rubix core",
	"This command will be used to ping the peer",
	"This command will add bootstrap peers to the configuration",
	"This command will remove bootstrap peers from the configuration",
	"This command will remove all bootstrap peers from the configuration",
	"This command will get all bootstrap peers from the configuration",
	"This command will create DID",
	"This command will get all DID address",
	"This command will add quorum list to node",
	"This command will get all quorum list from node",
	"This command will delete all quorum list from node",
	"This command will setup node as quorum",
	"This command will generate test RBT token",
	"This command will generate mainnet RBT tokens",
	"This command will give the RBT balance",
	"This command will register DID peer map across the network",
	"This command will setup the DID with peer",
	"This command will shutdown the rubix node",
	"This command will subscribe to a smart contract token",
	"This command will create NFT",
	"This command will generate a smart contract token",
	"This command will fetch a smart contract token",
	"This command will fetch the peer ID of the node",
	"This command is to add the peer details manually",
	"This command will check the quorum status",
	"This command will generate a faucet RBT token",
	"This command will create FT",
	"This command will give the balance of FTs",
	"This command will subscribe NFT",
	"This command will fetch NFT",
	"This command will add peer details from the explorer",
	"This command will sign an arbitrary message with the signer DID",
	"This command will verify a signed message",
	"This command will remove a stale DID",
	"This command will give the DID balance",
	"This command will initialise the node",
}

type Command struct {
	cfg                          types.RubixConfig
	c                            *client.Client
	encKey                       string
	start                        bool
	node                         uint
	nodeConfigPath               string
	logFile                      string
	logLevel                     string
	testnet                      bool
	mainnet                      bool
	localnet                     bool
	testNetKey                   string
	addr                         string
	port                         string
	peerID                       string
	peers                        []string
	log                          logger.Logger
	forcePWD                     bool
	privPWD                      string
	quorumPWD                    string
	pubKeyFile                   string
	srvName                      string
	storageType                  string
	dbName                       string
	dbType                       string
	dbAddress                    string
	dbPort                       uint64
	dbUserName                   string
	dbPassword                   string
	senderAddr                   string
	receiverAddr                 string
	rbtAmount                    float64
	transComment                 string
	quorumType                   int
	numTokens                    int
	startIndex                   int
	enableAuth                   bool
	did                          string
	token                        string
	arbitaryMode                 bool
	tokenList                    string
	batchID                      string
	fileMode                     bool
	file                         string
	userID                       string
	userInfo                     string
	timeout                      time.Duration
	txnID                        string
	role                         string
	deployerAddr                 string
	binaryCodePath               string
	rawCodePath                  string
	smartContractToken           string
	publishType                  int
	smartContractData            string
	executorAddr                 string
	latest                       bool
	quorumAddr                   string
	links                        []string
	mnemonic                     string
	ChildPath                    int
	TokenState                   string
	pinningAddress               string
	blockCount                   int
	smartContractChainValidation bool
	levelofToken                 int
	metadata                     string
	artifact                     string
	nft                          string
	nftData                      string
	ftName                       string
	ftCount                      int
	creatorDID                   string
	defaultSetup                 bool
	nftValue                     float64
	ftNumStartIndex              int
	message                      string
	signature                    string
	signerDID                    string
	enableTrustedNetwork         bool
	disableTrustedNetwork        bool
	backupDB                     bool
	fullNode                     bool
	dumpFullnodeTokenChain       bool
	assetType                    string
	enableDeExp                  bool
	deExpURL                     string
	operationType                int
	faucetURL                    string
}

func showVersion() {
	fmt.Printf("\n****************************************\n\n")
	fmt.Printf("Rubix Core Version  : %s\n", version)
	fmt.Printf("Current Commit      : %s\n", currentCommit)
	fmt.Printf("Previous Commit     : %s\n", previousCommit)
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

// Get preferred outbound ip of this machine
func (cmd *Command) getURL(url string) string {
	// No IP address present
	if strings.Contains(url, "://:") {
		conn, err := net.Dial("udp", "8.8.8.8:80")
		if err != nil {
			return url
		}
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		outIp := localAddr.IP.String()
		s := strings.Split(url, "://:")
		url = s[0] + "://" + outIp + ":" + s[1]
	}
	cmd.log.Info("Swagger URL : " + url + "/swagger/index.html")
	return url
}

func (cmd *Command) init() {
	if err := config.CreateConfigFileFromTemplate(cmd.nodeConfigPath); err != nil {
		cmd.log.Error(fmt.Sprintf("failed to create config.toml file at path: %v, err: %v", cmd.nodeConfigPath, err))
		return
	}
}

func (cmd *Command) runApp() {
	userConfig, err := config.ParseConfigFromPath(cmd.nodeConfigPath)
	if err != nil {
		cmd.log.Error(fmt.Sprintf("failed to parse config.toml, err: %v", err))
		return
	}

	rubixConfig, err := config.CreateRubixConfigFromUserConfig(userConfig, cmd.nodeConfigPath)
	if err != nil {
		cmd.log.Error(fmt.Sprintf("failed to get rubix config, err: %v", err))
	}
	cmd.cfg = rubixConfig
	cmd.cfg.CfgData.Ports = cmd.cfg.PortConfig

	if cmd.disableTrustedNetwork {
		cmd.cfg.TrustedNetwork = false
		cmd.log.Info("Trusted network mode explicitly disabled via -disableTrustedNetwork flag")
	} else {
		// Trusted network is enabled by default
		cmd.cfg.TrustedNetwork = true
		cmd.log.Info("Trusted network mode enabled (default)")
	}

	sc := make(chan bool, 1)

	rubixCore, err := core.NewCore(
		&cmd.cfg,
		cmd.log,
		userConfig.Core.NetworkMode,
		cmd.fullNode,
		cmd.faucetURL,
	)
	if err != nil {
		cmd.log.Error(err.Error())
		cmd.log.Error("failed to create core")
		return
	}

	serverConfig := &server.Config{
		Config: srvcfg.Config{
			HostAddress: "0.0.0.0",
			HostPort:    fmt.Sprintf("%d", cmd.cfg.PortConfig.RubixServerPort),
			Production:  "false",

			DBName:     cmd.cfg.DBConfig.DBName,
			DBAddress:  cmd.cfg.DBConfig.Host,
			DBPort:     uint64(cmd.cfg.DBConfig.Port),
			DBUserName: cmd.cfg.DBConfig.Username,
			DBPassword: cmd.cfg.DBConfig.Password,
		},
		EnableAuth: cmd.enableAuth,
	}

	s, err := server.NewServer(rubixCore, serverConfig, cmd.log, cmd.start, sc, cmd.timeout)
	if err != nil {
		cmd.log.Error("Failed to create server")
		return
	}
	s.EnableSWagger(cmd.getURL(s.GetServerURL()))
	cmd.log.Info("Core version : " + version)
	cmd.log.Info("Starting server...")
	go s.Start()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	signal.Notify(ch, syscall.SIGINT)
	select {
	case <-ch:
		// // signal ticker goroutine to stop
		// close(sc) // closing sc will unblock the ticker goroutine's case <-sc:
	case <-sc:
	}

	s.Shutdown()
	cmd.log.Info("Shutting down...")
}

func (cmd *Command) validateOptions() bool {
	if cmd.nodeConfigPath == "" {
		cmd.nodeConfigPath = "./"
	}
	if !strings.HasPrefix(cmd.nodeConfigPath, "\\") {
		if !strings.HasPrefix(cmd.nodeConfigPath, "/") {
			cmd.nodeConfigPath = cmd.nodeConfigPath + "/"
		}
	}
	_, err := os.Stat(cmd.nodeConfigPath)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.MkdirAll(cmd.nodeConfigPath, os.ModeDir|os.ModePerm)
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
	var timeout int
	var links string

	flag.StringVar(&cmd.nodeConfigPath, "p", "./", "Working directory path")
	flag.StringVar(&cmd.logFile, "logFile", "", "Log file name")
	flag.StringVar(&cmd.logLevel, "logLevel", "debug", "Log level")
	flag.UintVar(&cmd.node, "n", 0, "Node number")
	flag.StringVar(&cmd.encKey, "k", "TestKeyBasic#2022", "Config file encryption key")
	flag.BoolVar(&cmd.start, "s", false, "Start the core")
	flag.BoolVar(&cmd.testnet, "testnet", false, "Run as testnet")
	flag.BoolVar(&cmd.localnet, "localnet", false, "Run as local network")
	flag.BoolVar(&cmd.mainnet, "mainnet", false, "Run as main network")
	flag.StringVar(&cmd.testNetKey, "testNetKey", "testswarm.key", "Test net key")
	flag.StringVar(&cmd.addr, "addr", "localhost", "Server/Host Address")
	flag.StringVar(&cmd.port, "port", "20000", "Server/Host port")
	flag.StringVar(&cmd.peerID, "peerID", "", "Peerd ID")
	flag.StringVar(&peers, "peers", "", "Bootstrap peers, mutiple peers will be seprated by comma")
	flag.IntVar(&cmd.ChildPath, "ChildPath", 0, "BIP child Path")
	flag.BoolVar(&cmd.forcePWD, "fp", false, "Force password entry")
	flag.StringVar(&cmd.privPWD, "privPWD", "mypassword", "Private key password")
	flag.StringVar(&cmd.quorumPWD, "quorumPWD", "mypassword", "Quorum key password")
	flag.StringVar(&cmd.mnemonic, "mnemonic", "", "Mnemonic keys")
	flag.StringVar(&cmd.pubKeyFile, "publicKey", "", "Public key")
	flag.StringVar(&cmd.srvName, "srvName", "explorer_service", "Service name")
	flag.StringVar(&cmd.storageType, "storageType", constants.DBType_PostgreSQL, "Storage type")
	flag.StringVar(&cmd.dbName, "dbName", "rubix", "Service database name")
	flag.StringVar(&cmd.dbType, "dbType", constants.DBType_PostgreSQL, "DB Type, supported database are: postgres")
	flag.StringVar(&cmd.dbAddress, "dbAddress", constants.DefaultPostgresHost, "Database address")
	flag.Uint64Var(&cmd.dbPort, "dbPort", constants.DefaultPostgresPort, "Database port number")
	flag.StringVar(&cmd.dbUserName, "dbUsername", constants.DefaultPostgresUsername, "Database username")
	flag.StringVar(&cmd.dbPassword, "dbPassword", constants.DefaultPostgresPassword, "Database password")
	flag.StringVar(&cmd.senderAddr, "senderAddr", "", "Sender address")
	flag.StringVar(&cmd.receiverAddr, "receiverAddr", "", "Receiver address")
	flag.Float64Var(&cmd.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	flag.StringVar(&cmd.transComment, "transComment", "", "Transaction comment")
	flag.IntVar(&cmd.quorumType, "quorumType", 2, "Quorum type")
	flag.IntVar(&cmd.numTokens, "numTokens", 1, "Number of tokens")
	flag.IntVar(&cmd.startIndex, "startIndex", 0, "startIndex to generate test rbt tokens locally")
	flag.StringVar(&cmd.did, "did", "", "DID")
	flag.BoolVar(&cmd.enableAuth, "enableAuth", false, "Enable authentication")
	flag.BoolVar(&cmd.arbitaryMode, "arbitaryMode", false, "Enable arbitary mode")
	flag.StringVar(&cmd.tokenList, "tokenList", "tokens.txt", "Token lis")
	flag.StringVar(&cmd.token, "token", "", "Token name")
	flag.StringVar(&cmd.batchID, "bid", "batchID1", "Batch ID")
	flag.BoolVar(&cmd.fileMode, "fmode", false, "File mode")
	flag.StringVar(&cmd.file, "file", "file.txt", "File to be uploaded")
	flag.StringVar(&cmd.userID, "uid", "testuser", "User ID for token creation")
	flag.StringVar(&cmd.userInfo, "uinfo", "", "User info for token creation")
	flag.IntVar(&timeout, "timeout", 0, "Timeout for the server")
	flag.StringVar(&cmd.txnID, "txnID", "", "Transaction ID")
	flag.StringVar(&cmd.role, "role", "", "Sender/Receiver")
	// flag.StringVar(&cmd.grpcAddr, "grpcAddr", "localhost", "GRPC server address")
	// flag.IntVar(&cmd.grpcPort, "grpcPort", 10500, "GRPC server port")
	// flag.BoolVar(&cmd.grpcSecure, "grpcSecure", false, "GRPC enable security")
	flag.StringVar(&cmd.deployerAddr, "deployerAddr", "", "Smart contract Deployer Address")
	flag.StringVar(&cmd.binaryCodePath, "binCode", "", "Binary code path")
	flag.StringVar(&cmd.rawCodePath, "rawCode", "", "Raw code path")
	flag.StringVar(&cmd.smartContractToken, "sct", "", "Smart contract token")
	flag.IntVar(&cmd.publishType, "pubType", 0, "Smart contract event publishing type(Deploy & Execute)")
	flag.StringVar(&cmd.smartContractData, "sctData", "data", "Smart contract execution info")
	flag.StringVar(&cmd.executorAddr, "executorAddr", "", "Smart contract Executor Address")
	flag.BoolVar(&cmd.latest, "latest", false, "flag to set latest")
	flag.StringVar(&cmd.quorumAddr, "quorumAddr", "", "Quorum Node Address to check the status of the Quorum")
	flag.StringVar(&links, "links", "", "Explorer url")
	flag.StringVar(&cmd.TokenState, "tokenstatehash", "", "Give Token State Hash to check state")
	flag.StringVar(&cmd.pinningAddress, "pinningAddress", "", "Pinning address")
	flag.IntVar(&cmd.blockCount, "blockCount", 0, "Number of blocks of the tokenchain to validate")
	flag.BoolVar(&cmd.smartContractChainValidation, "sctValidation", false, "Validate smart contract token chain")
	flag.IntVar(&cmd.levelofToken, "level", 0, "Level for which tokens need to be generated")
	flag.StringVar(&cmd.nft, "nft", "", "NFT id")
	flag.StringVar(&cmd.metadata, "metadata", "", "NFT metadata")
	flag.StringVar(&cmd.artifact, "artifact", "", "NFT artifact")
	flag.StringVar(&cmd.nftData, "nftData", "", "The nft data")
	flag.StringVar(&cmd.ftName, "ftName", "", "Name of FT to be created")
	flag.IntVar(&cmd.ftCount, "ftCount", 0, "Number of FTs to be created")
	flag.StringVar(&cmd.creatorDID, "creatorDID", "", "DID of creator of FT")
	flag.BoolVar(&cmd.defaultSetup, "defaultSetup", false, "Add Faucet Quorums")
	flag.Float64Var(&cmd.nftValue, "nftValue", 0.0, "Value of the NFT")
	flag.IntVar(&cmd.ftNumStartIndex, "ftStartIndex", 0, "Start index of the FTs to be created")
	flag.StringVar(&cmd.message, "message", "", "Value to be signed on")
	flag.StringVar(&cmd.signature, "signature", "", "signature to be verified")
	flag.StringVar(&cmd.signerDID, "signerdid", "", "DID of the signer")
	flag.BoolVar(&cmd.enableTrustedNetwork, "enableTrustedNetwork", true, "Enable trusted network mode (skips DHT checks) - enabled by default")
	flag.BoolVar(&cmd.disableTrustedNetwork, "disableTrustedNetwork", false, "Disable trusted network mode to enable full DHT checks")
	flag.BoolVar(&cmd.backupDB, "backupDB", false, "Create backup of database before starting node")
	flag.BoolVar(&cmd.fullNode, "fullnode", false, "receive all published transactions and tokenchain details")
	flag.BoolVar(&cmd.dumpFullnodeTokenChain, "fullnodetoken", false, "dump tokenchain from fullnode storage")
	flag.StringVar(&cmd.assetType, "assettype", "rbt", "DID of the signer")
	flag.BoolVar(&cmd.enableDeExp, "deexp", false, "Host a decentralized explorer from fullnode")
	flag.StringVar(&cmd.deExpURL, "deexpURL", "", "Decentralized explorer Server URL")
	flag.IntVar(&cmd.operationType, "operationType", 0, "this defines the underlying transaction type")
	flag.StringVar(&cmd.faucetURL, "faucetURL", "", "Faucet Server URL")

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

	if links != "" {
		links = strings.ReplaceAll(links, " ", "")
		cmd.links = strings.Split(links, ",")
	}

	cmd.timeout = time.Duration(timeout) * time.Minute

	if !cmd.validateOptions() {
		fmt.Println("Validate options failed")
		return
	}

	if cmd.logFile == "" {
		cmd.logFile = filepath.Join(cmd.nodeConfigPath, "log.txt")
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

	cmd.c, err = client.NewClient(&srvcfg.Config{ServerAddress: cmd.addr, ServerPort: cmd.port}, cmd.log, cmd.timeout)
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
	case GenerateLocalRBTCmd:
		cmd.GenerateLocalRBT()
	case GenerateMainnetRBTCmd:
		cmd.GenerateMainnetRBT()
	case GetRBTBalanceCmd:
		cmd.GetRBTBalance()
	case RegsiterDIDCmd:
		cmd.RegsiterDIDCmd()
	case SetupDIDCmd:
		cmd.SetupDIDCmd()
	case ShutDownCmd:
		cmd.ShutDownCmd()
	case SubscribeContractCmd:
		cmd.SubscribeContract()
	case CreateNFTCmd:
		cmd.createNFT()
	case GenerateSmartContractToken:
		cmd.generateSmartContractToken()
	case FetchSmartContract:
		cmd.fetchSmartContract()
	case GetPeerID:
		cmd.peerIDCmd()
	case CheckQuorumStatusCmd:
		cmd.checkQuorumStatus()
	case AddPeerDetailsCmd:
		cmd.AddPeerDetails()
	case GenerateFaucetTestRBTCmd:
		cmd.GenerateFaucetTestRBT()
	case CreateFTCmd:
		cmd.createFT()
	case GetFTBalanceCmd:
		cmd.getFTinfo()
	case SubscribeNFTCmd:
		cmd.SubscribeNFT()
	case FetchNftCmd:
		cmd.fetchNFT()
	case AddPeerDetailsFromExplorer:
		cmd.addPeerDetailsFromExplorer()
	case ArbitrarySignCmd:
		cmd.ArbitrarySign()
	case VerifySignatureCmd:
		cmd.SignVerification()
	case RemoveStaleDIDCmd:
		cmd.RemoveStaleDID()
	case GetDIDBalanceCmd:
		cmd.GetDIDBalance()
	case InitCmd:
		cmd.init()

	default:
		cmd.log.Error("Invalid command")
	}
}

func getpassword(msg string) (string, error) {
	fmt.Print(msg)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}
