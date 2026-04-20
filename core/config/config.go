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
	"/ip4/161.35.169.251/tcp/4001/p2p/12D3KooWPhZEYEw4jG3kSRuwgMEHcVt7KMkm1ui2ddu4fgSgwvDq", 
	"/ip4/103.127.158.120/tcp/4001/p2p/12D3KooWSQ94HRDzFf6W2rp7P8gzP6efZQHTaSU8uaQjskVBHiWP", 
	"/ip4/172.104.191.191/tcp/4001/p2p/12D3KooWFudnWZY1v1m4YXCzDWZSbNt7nvf5F42uzM6vErZ4NwqJ",
]
testnet_bootstrap_nodes = [
	"/ip4/103.209.145.177/tcp/4001/p2p/12D3KooWD8Rw7Fwo4n7QdXTCjbh6fua8dTqjXBvorNz3bu7d9xMc", 
	"/ip4/98.70.52.158/tcp/4001/p2p/12D3KooWQyWFABF3CKFnzX85hf5ZwrT5zPsy4rWHdGPZ8bBpRVCK", 
	"/ip4/20.244.16.143/tcp/4001/p2p/12D3KooWAydFDJeSW5qupmp3AjRxc82Dq1AnjfJT1zwy4hg2TuNn", 
	"/ip4/40.81.232.217/tcp/4001/p2p/12D3KooWK6V21GQotbub3cfgb5qAK1uUoUGPexf3vsLqw6yBJfen",
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
