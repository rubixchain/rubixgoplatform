package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
)

// ---------------------------------------------------------------------------------------------------------- //

const userConfigTemplate = `
[core]
node_index = 0
network_mode = "mainnet|testnet|localnet"
enable_trusted_network = false

[db]
host = "localhost"
username = "rubix"
password = "rubixpass"
db_name = "rubix"

[db.config]
max_connections = 50
min_connections = 10
max_connection_lifetime_seconds = 60
max_connection_idletime_seconds = 30
statement_timeout_seconds = 20

[ipfs]
mainnet_bootstrap_nodes = [
	"/ip4/172.188.67.118/tcp/4001/p2p/12D3KooWN9Nrg2DmY52uc8ihvjPTFEnJKMdveztDFVgxRyoD6QyE",
	"/ip4/103.127.158.120/tcp/4001/p2p/12D3KooWJSckNDq5CD8b3bs5WSJN6GApVw547i4ye19C7R99LLWH",
  	"/ip4/161.35.169.251/tcp/4001/p2p/12D3KooWMnBFGSvnfa82J42FnGxcBnSNEsGpt2G5iq3C79a3Hm7H",
  	"/ip4/172.104.191.191/tcp/4001/p2p/12D3KooWFudnWZY1v1m4YXCzDWZSbNt7nvf5F42uzM6vErZ4NwqJ",
]
testnet_bootstrap_nodes = [
	"/ip4/103.209.145.177/tcp/4011/p2p/12D3KooWKAHAYEjjckeWi2s3oCnvkbTJX3x6fV3HZbjMEVzMfJeL",
	"/ip4/103.209.145.177/tcp/4001/p2p/12D3KooWNF5g1G6QHa2Xjbnsm5N1e4FKompLbk3dRcSgZkVzmFsh",
  	"/ip4/98.70.52.158/tcp/4001/p2p/12D3KooWNrfpqpSWi4N1WL9Xv8j87hm7nBMqJ3vWWp3qJ3d2MJp8",
]
localnet_bootstrap_nodes = []
`

// LegacyConfigData holds nested config fields from the SQLite-era config.
// TODO(phase11-cleanup): remove along with Config when legacy tools are updated.
type LegacyConfigData struct {
	TrustedNetwork bool `json:"trusted_network"`
}

// Config is a legacy top-level node configuration struct (SQLite/BoltDB era).
// Preserved for grpcclient and tools compatibility; new code uses types.RubixConfig instead.
// TODO(phase11-cleanup): remove when grpcclient/tools are fully updated.
type Config struct {
	NodeAddress string           `json:"node_address"`
	NodePort    string           `json:"node_port"`
	DirPath     string           `json:"dir_path"`
	CfgData     LegacyConfigData `json:"cfg_data"`
}

// StorageConfig defines legacy storage configuration (SQLite-era; used by client/server setup endpoints).
// TODO(phase11-cleanup): remove when SetupDB endpoint is fully retired.
type StorageConfig struct {
	StorageType int    `json:"stroage_type"`
	DBName      string `json:"db_name"`
	DBAddress   string `json:"db_address"`
	DBPort      string `json:"db_port"`
	DBType      string `json:"db_type"`
	DBUserName  string `json:"db_user_name"`
	DBPassword  string `json:"db_password"`
}

// ServiceConfig defines legacy service configuration.
// TODO(phase11-cleanup): remove when SetupService endpoint is fully retired.
type ServiceConfig struct {
	ServiceSettings string `json:"service_settings"`
	ServiceName     string `json:"service_name"`
	DBName          string `json:"db_name"`
	DBAddress       string `json:"db_address"`
	DBPort          string `json:"db_port"`
	DBType          string `json:"db_type"`
	DBUserName      string `json:"db_user_name"`
	DBPassword      string `json:"db_password"`
}

func ParseConfigFromPath(configPath string) (types.UserConfig, error) {
	configFilePath := path.Join(configPath, "config.toml")
	configDataBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return types.UserConfig{}, fmt.Errorf("failed to read config.toml file from path: %v, err: %v", configFilePath, err)
	}

	var rubixConfig types.UserConfig
	if err := toml.Unmarshal(configDataBytes, &rubixConfig); err != nil {
		return types.UserConfig{}, fmt.Errorf("failed to marshal config, err: %v", err)
	}

	return rubixConfig, nil
}

func CreateConfigFileFromTemplate(configPath string) error {
	configFilePath := path.Join(configPath, "config.toml")
	if _, err := os.Stat(configFilePath); err == nil {
		return nil
	}

	err := os.WriteFile(configFilePath, []byte(userConfigTemplate), 0644)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	return nil
}

func CreateRubixConfigFromUserConfig(userConfig types.UserConfig, nodeDir string) (types.RubixConfig, error) {
	var rubixConfig types.RubixConfig

	rubixConfig.NodeDir = nodeDir

	// Set DID Directory based on the network
	var networkDirName string
	switch userConfig.Core.NetworkMode {
	case constants.NetworkMode_Mainnet:
		networkDirName = constants.NetworkMode_Mainnet
	case constants.NetworkMode_Testnet:
		networkDirName = constants.NetworkMode_Testnet
	case constants.NetworkMode_Localnet:
		networkDirName = constants.NetworkMode_Localnet
	default:
		return types.RubixConfig{}, fmt.Errorf("failed while creating RubixConfig, invalid network type: %v", userConfig.Core.NetworkMode)
	}

	rubixConfig.NetworkDir = filepath.Join(nodeDir, networkDirName)
	rubixConfig.DidDir = filepath.Join(rubixConfig.NetworkDir, "dids")
	rubixConfig.NFTDir = filepath.Join(rubixConfig.NetworkDir, "nfts")
	rubixConfig.SmartContractDir = filepath.Join(rubixConfig.NetworkDir, "smart_contracts")
	rubixConfig.TrustedNetwork = userConfig.Core.EnableTrustedNetwork
	rubixConfig.PortConfig.IPFSPort = (constants.IPFSPort + uint16(userConfig.Core.NodeIndex))
	rubixConfig.PortConfig.SendPort = (constants.SendPort + uint16(userConfig.Core.NodeIndex))
	// ReceiverPort is spaced by MaxPeerConn so the derived Listener (+10) and
	// PeerManager (+11) ports don't collide with the next node's ReceiverPort.
	rubixConfig.PortConfig.ReceiverPort = constants.RecvPort + (constants.MaxPeerConn * uint16(userConfig.Core.NodeIndex))
	rubixConfig.PortConfig.SwarmPort = (constants.SwarmPort + uint16(userConfig.Core.NodeIndex))
	rubixConfig.PortConfig.IPFSAPIPort = (constants.IPFSAPIPort + uint16(userConfig.Core.NodeIndex))
	rubixConfig.PortConfig.RubixServerPort = (constants.RubixServerPort + uint16(userConfig.Core.NodeIndex))
	if userConfig.Db.Port != 0 {
		rubixConfig.DBConfig.Port = userConfig.Db.Port
	} else {
		rubixConfig.DBConfig.Port = (int(constants.PostgresBasePort) + int(userConfig.Core.NodeIndex))
	}

	rubixConfig.MainnetBootstrap = userConfig.Ipfs.MainnetBootstrapNodes
	rubixConfig.TestnetBootstrap = userConfig.Ipfs.TestnetBootstrapNodes
	rubixConfig.LocalnetBootStrap = userConfig.Ipfs.LocalnetBootstrapNodes

	// Postgres DB Config
	rubixConfig.DBConfig.DBName = userConfig.Db.DBName
	rubixConfig.DBConfig.Host = userConfig.Db.Host
	rubixConfig.DBConfig.Password = userConfig.Db.Password
	rubixConfig.DBConfig.Username = userConfig.Db.Username

	// Postgres DB Params
	rubixConfig.DBConfig.Params.MaxConnectionIdletimeSeconds = userConfig.Db.Params.MaxConnectionIdletimeSeconds
	rubixConfig.DBConfig.Params.MaxConnectionLifetimeSeconds = userConfig.Db.Params.MaxConnectionLifetimeSeconds
	rubixConfig.DBConfig.Params.MaxConnections = userConfig.Db.Params.MaxConnections
	rubixConfig.DBConfig.Params.MinConnections = userConfig.Db.Params.MinConnections
	rubixConfig.DBConfig.Params.StatementTimeoutSeconds = userConfig.Db.Params.StatementTimeoutSeconds

	return rubixConfig, nil
}
