package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/protos"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	ConfigFile string = "api_config.json"
)

const (
	version string = "0.0.8"
)
const (
	VersionCmd    string = "-v"
	HelpCmd       string = "-h"
	CreateDIDCmd  string = "createdid"
	GetBalance    string = "getbalance"
	GenerateRBT   string = "generaterbt"
	GettAllTokens string = "getalltokens"
)

var commands = []string{VersionCmd,
	HelpCmd,
	CreateDIDCmd,
	GetBalance,
	GenerateRBT,
	GettAllTokens,
}
var commandsHelp = []string{"To get tool version",
	"To get help",
	"To run the rubix core",
	"This command will create DID",
	"This command will get the DID balance",
	"This command will generate RBT",
	"This commadn will get all tokens",
}

type Command struct {
	cfg          config.Config
	c            protos.RubixServiceClient
	ctx          context.Context
	accessToken  string
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
	storageType  int
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
	batchID      string
	fileMode     bool
	file         string
	userID       string
	userInfo     string
	timeout      time.Duration
	txnID        string
	role         string
	date         time.Time
	grpcAddr     string
	grpcPort     int
	grpcSecure   bool
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

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := ioutil.ReadFile("ca-cert.pem")
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to add server CA's certificate")
	}

	// Create the credentials and return it
	config := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(config), nil
}

func runCommand() {
	cmd := &Command{}
	var peers string
	var timeout int

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
	flag.IntVar(&cmd.storageType, "storageType", storage.StorageDBType, "Storage type")
	flag.StringVar(&cmd.dbName, "dbName", "ServiceDB", "Service database name")
	flag.StringVar(&cmd.dbType, "dbType", "SQLServer", "DB Type, supported database are SQLServer, PostgressSQL, MySQL & Sqlite3")
	flag.StringVar(&cmd.dbAddress, "dbAddress", "localhost", "Database address")
	flag.StringVar(&cmd.dbPort, "dbPort", "1433", "Database port number")
	flag.StringVar(&cmd.dbUserName, "dbUsername", "sa", "Database username")
	flag.StringVar(&cmd.dbPassword, "dbPassword", "password", "Database password")
	flag.StringVar(&cmd.senderAddr, "senderAddr", "", "Sender address")
	flag.StringVar(&cmd.receiverAddr, "receiverAddr", "", "Receiver address")
	flag.Float64Var(&cmd.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	flag.StringVar(&cmd.transComment, "transComment", "", "Transaction comment")
	flag.IntVar(&cmd.transType, "transType", 2, "Transaction type")
	flag.IntVar(&cmd.numTokens, "numTokens", 1, "Number of tokens")
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
	flag.StringVar(&cmd.grpcAddr, "grpcAddr", "localhost", "GRPC server address")
	flag.IntVar(&cmd.grpcPort, "grpcPort", 10500, "GRPC server port")
	flag.BoolVar(&cmd.grpcSecure, "grpcSecure", false, "GRPC enable security")

	if len(os.Args) < 2 {
		fmt.Println("Invalid Command")
		showHelp()
		return
	}

	cmdName := os.Args[1]

	os.Args = os.Args[1:]

	flag.Parse()
	var err error
	var conn *grpc.ClientConn
	addr := fmt.Sprintf(cmd.grpcAddr+":%d", cmd.grpcPort)
	if cmd.grpcSecure {
		tlsCredentials, err := loadTLSCredentials()
		if err != nil {
			fmt.Printf("cannot load TLS credentials: %v", err)
		}
		conn, err = grpc.Dial(addr, grpc.WithTransportCredentials(tlsCredentials))
		if err != nil {
			fmt.Printf("Failed to dial")
			return
		}
	} else {
		conn, err = grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			fmt.Printf("Failed to dial")
			return
		}
	}
	defer conn.Close()
	cmd.c = protos.NewRubixServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	cmd.ctx = ctx

	switch cmdName {
	case VersionCmd:
		showVersion()
	case HelpCmd:
		showHelp()
	case CreateDIDCmd:
		cmd.CreateDID()
	case GetBalance:
		cmd.GetBalance()
	case GenerateRBT:
		cmd.GenerateRBT()
	case GettAllTokens:
		cmd.GettAllTokens()
	}

}
