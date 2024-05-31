package command

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/server"
	"github.com/rubixchain/rubixgoplatform/wrapper/apiconfig"
	srvcfg "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type CommandConfig struct {
	cfg                config.Config
	c                  *client.Client
	sc                 *contract.Contract
	encKey             string
	start              bool
	node               uint
	runDir             string
	logFile            string
	logLevel           string
	cfgFile            string
	testNet            bool
	testNetKey         string
	addr               string
	port               string
	peerID             string
	peers              []string
	log                logger.Logger
	didRoot            bool
	didType            int
	didSecret          string
	forcePWD           bool
	privPWD            string
	quorumPWD          string
	imgFile            string
	didImgFile         string
	privImgFile        string
	pubImgFile         string
	privKeyFile        string
	pubKeyFile         string
	quorumList         string
	srvName            string
	storageType        int
	dbName             string
	dbType             string
	dbAddress          string
	dbPort             string
	dbUserName         string
	dbPassword         string
	senderAddr         string
	receiverAddr       string
	rbtAmount          float64
	transComment       string
	transType          int
	numTokens          int
	enableAuth         bool
	did                string
	token              string
	arbitaryMode       bool
	tokenList          string
	batchID            string
	fileMode           bool
	file               string
	userID             string
	userInfo           string
	timeout            time.Duration
	txnID              string
	role               string
	date               time.Time
	grpcAddr           string
	grpcPort           int
	grpcSecure         bool
	deployerAddr       string
	binaryCodePath     string
	rawCodePath        string
	schemaFilePath     string
	smartContractToken string
	newContractBlock   string
	publishType        int
	smartContractData  string
	executorAddr       string
	latest             bool
	quorumAddr         string
	links              []string
	mnemonicFile       string
	ChildPath          int
}

func (cmd *CommandConfig) getURL(url string) string {
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

func (cmd *CommandConfig) validateOptions() error {
	if cmd.runDir == "" {
		cmd.runDir = "./"
	}
	if !strings.HasPrefix(cmd.runDir, "\\") {
		if !strings.HasPrefix(cmd.runDir, "/") {
			cmd.runDir = cmd.runDir + "/"
		}
	}
	_, err := os.Stat(cmd.runDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(cmd.runDir, os.ModeDir|os.ModePerm)
		if err != nil || os.IsExist(err) {
			return err
		}
	} else {
		return err
	}

	return nil
}

func getpassword(msg string) (string, error) {
	fmt.Print(msg)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

var (
	commandCfg *CommandConfig = &CommandConfig{}
	peers      string
	timeout    int

	ConfigFile string = "api_config.json"
)

var rootCmd = &cobra.Command{
	Use:                        "rubixgoplatform",
	Short:                      "Rubix Blockchain Platform CLI",
	Long:                       `Rubix Blockchain Platform CLI`,
	SuggestionsMinimumDistance: 2,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if peers != "" {
			peers = strings.ReplaceAll(peers, " ", "")
			commandCfg.peers = strings.Split(peers, ",")
		}

		commandCfg.timeout = time.Duration(timeout) * time.Minute

		if err := commandCfg.validateOptions(); err != nil {
			fmt.Printf("Validate options failed, error: %v\n", err)
			os.Exit(1)
		}

		if commandCfg.logFile == "" {
			commandCfg.logFile = commandCfg.runDir + "log.txt"
		}

		level := logger.Debug

		fp, err := os.OpenFile(commandCfg.logFile,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}

		switch strings.ToLower(commandCfg.logLevel) {
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

		commandCfg.log = logger.New(logOptions)
		
		// Get addr and port
		commandCfg.addr, err = cmd.Root().PersistentFlags().GetString("addr")
		if err != nil {
			return err
		}
		commandCfg.port, err = cmd.Root().PersistentFlags().GetString("port")
		if err != nil {
			return err
		}

		commandCfg.c, err = client.NewClient(&srvcfg.Config{ServerAddress: commandCfg.addr, ServerPort: commandCfg.port}, commandCfg.log, commandCfg.timeout)
		if err != nil {
			commandCfg.log.Error("Failed to create client")
			return err
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			cmd.Help()
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Global Flags
	rootCmd.PersistentFlags().StringVar(&commandCfg.addr, "addr", "localhost", "Node address")
	rootCmd.PersistentFlags().StringVar(&commandCfg.port, "port", "20000", "Node port")

	// Removes the default `completion` command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(
		runApplication(commandCfg),
		didCommandGroup(commandCfg),
		bootstrapCommandGroup(commandCfg),
		configCommandGroup(commandCfg),
		nodeGroup(commandCfg),
		explorerCommandGroup(commandCfg),
		quorumCommandGroup(commandCfg),
		txCommandGroup(commandCfg),
		chainDumpCommandGroup(commandCfg),
		versionCmd(),
	)

	// Disables the default help command. Not to be confused with the help flag (`--help` or `-h`) 
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}

func runApplication(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run Rubix Core",
		Long:    "Run Rubix Core",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(cmdCfg.runDir, os.ModePerm); err != nil {
				fmt.Println("Error creating directories:", err)
				return err
			}

			core.InitConfig(path.Join(cmdCfg.runDir, cmdCfg.cfgFile), cmdCfg.encKey, uint16(cmdCfg.node))
			err := apiconfig.LoadAPIConfig(path.Join(cmdCfg.runDir, cmdCfg.cfgFile), cmdCfg.encKey, &cmdCfg.cfg)
			if err != nil {
				cmdCfg.log.Error("Configfile is either currupted or cipher is wrong", "err", err)
				return err
			}

			// Override directory path
			cmdCfg.cfg.DirPath = cmdCfg.runDir
			sc := make(chan bool, 1)
			c, err := core.NewCore(&cmdCfg.cfg, path.Join(cmdCfg.runDir, cmdCfg.cfgFile), cmdCfg.encKey, cmdCfg.log, cmdCfg.testNet, cmdCfg.testNetKey, cmdCfg.arbitaryMode)
			if err != nil {
				cmdCfg.log.Error("failed to create core")
				return err
			}

			addr := fmt.Sprintf(cmdCfg.grpcAddr+":%d", cmdCfg.grpcPort)
			scfg := &server.Config{
				Config: srvcfg.Config{
					HostAddress: cmdCfg.cfg.NodeAddress,
					HostPort:    cmdCfg.cfg.NodePort,
					Production:  "false",
				},
				GRPCAddr:   addr,
				GRPCSecure: cmdCfg.grpcSecure,
			}
			scfg.EnableAuth = cmdCfg.enableAuth
			if cmdCfg.enableAuth {
				scfg.DBType = "Sqlite3"
				scfg.DBAddress =  path.Join(cmdCfg.cfg.DirPath, "rubix.db")
			}
			// scfg := &srvcfg.Config{
			// 	HostAddress: cmd.cfg.NodeAddress,
			// 	HostPort:    cmd.cfg.NodePort,
			// }
			s, err := server.NewServer(c, scfg, cmdCfg.log, cmdCfg.start, sc, cmdCfg.timeout)
			if err != nil {
				cmdCfg.log.Error("Failed to create server")
				return err
			}
			s.EnableSWagger(cmdCfg.getURL(s.GetServerURL()))
			cmdCfg.log.Info("Core version : " + version)
			cmdCfg.log.Info("Starting server...")
			go s.Start()

			ch := make(chan os.Signal, 1)
			signal.Notify(ch, syscall.SIGTERM)
			signal.Notify(ch, syscall.SIGINT)
			select {
			case <-ch:
			case <-sc:
			}
			s.Shutdown()
			cmdCfg.log.Info("Shutting down...")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.runDir, "p", "./", "working directory path")
	cmd.Flags().UintVar(&cmdCfg.node, "n", 0, "node number")
	cmd.Flags().BoolVar(&cmdCfg.start, "s", false, "Start the core")
	cmd.Flags().BoolVar(&cmdCfg.testNet, "testNet", false, "Run as test net")
	cmd.Flags().StringVar(&cmdCfg.testNetKey, "testNetKey", "testswarm.key", "Test net key")
	cmd.Flags().StringVar(&cmdCfg.cfgFile, "c", ConfigFile, "Configuration file for the core")
	cmd.Flags().StringVar(&cmdCfg.encKey, "k", "TestKeyBasic#2022", "Config file encryption key")
	cmd.Flags().BoolVar(&cmdCfg.arbitaryMode, "arbitaryMode", false, "Enable arbitary mode")
	cmd.Flags().IntVar(&cmdCfg.grpcPort, "grpcPort", 10500, "GRPC server port")
	cmd.Flags().BoolVar(&cmdCfg.grpcSecure, "grpcSecure", false, "GRPC enable security")

	return cmd
}
