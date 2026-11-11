package types

import "time"

type CoreConfig struct {
	NodeIndex            int    `toml:"node_index"`
	NetworkMode          string `toml:"network_mode"`
	EnableTrustedNetwork bool   `toml:"enable_trusted_network"`
	// LogRotation enables the periodic archival of the node log file. It is
	// optional and defaults to false, which keeps the logging behaviour of
	// the nodes which were deployed before this field existed unchanged.
	LogRotation bool `toml:"log_rotation"`
	// LogRotationPeriod is the rotation interval, e.g. "7d", "24h", "12h" or
	// "30m". It is optional and defaults to "7d".
	LogRotationPeriod string `toml:"log_rotation_period"`
}

type DBParams struct {
	MaxConnections               int `toml:"max_connections"`
	MinConnections               int `toml:"min_connections"`
	MaxConnectionLifetimeSeconds int `toml:"max_connection_lifetime_seconds"`
	MaxConnectionIdletimeSeconds int `toml:"max_connection_idletime_seconds"`
	StatementTimeoutSeconds      int `toml:"statement_timeout_seconds"`
}

type DBConfig struct {
	Host     string   `toml:"host"`
	Port     int      `toml:"port"`
	Username string   `toml:"username"`
	Password string   `toml:"password"`
	DBName   string   `toml:"db_name"`
	Params   DBParams `toml:"config"`
}

type IPFSUserConfig struct {
	MainnetBootstrapNodes  []string `toml:"mainnet_bootstrap_nodes"`
	TestnetBootstrapNodes  []string `toml:"testnet_bootstrap_nodes"`
	LocalnetBootstrapNodes []string `toml:"localnet_bootstrap_nodes"`
}

type UserConfig struct {
	Core CoreConfig     `toml:"core"`
	Db   DBConfig       `toml:"db"`
	Ipfs IPFSUserConfig `toml:"ipfs"`
}

// LogConfig holds the validated log rotation settings of the node.
type LogConfig struct {
	// Rotation reports whether the node log file has to be rotated.
	Rotation bool
	// RotationPeriod is the parsed `log_rotation_period`.
	RotationPeriod time.Duration
}

// IPFSRecoveryConfig defines IPFS recovery configuration
type IPFSRecoveryConfig struct {
	MaxRecoveries   int           `json:"max_recoveries"`   // Maximum recovery attempts
	RestartDelay    time.Duration `json:"restart_delay"`    // Delay between restart attempts
	HealthTimeout   time.Duration `json:"health_timeout"`   // Timeout for health checks
	MonitorInterval time.Duration `json:"monitor_interval"` // Health monitoring interval
}

// UnpledgePoolConfig defines unpledge worker pool configuration
type UnpledgePoolConfig struct {
	MaxWorkers       int           `json:"max_workers"`
	QueueSize        int           `json:"queue_size"`
	BatchSize        int           `json:"batch_size"`
	TokenConcurrency int           `json:"token_concurrency"`
	ShutdownTimeout  time.Duration `json:"shutdown_timeout"`
	EnableMetrics    bool          `json:"enable_metrics"`
}

type PortConfig struct {
	SendPort        uint16 `json:"sender_port"`
	ReceiverPort    uint16 `json:"receiver_port"`
	IPFSPort        uint16 `json:"ipfs_port"`
	SwarmPort       uint16 `json:"swarm_port"`
	IPFSAPIPort     uint16 `json:"ipfs_api_port"`
	RubixServerPort uint16 `json:"rubix_server_port"`
}

// RubixCfgData provides a nested accessor for config fields used by
// ipfs_health, ipfs_recovery, quorum_recv, token_state_validator, and unpledge_optimized.
type RubixCfgData struct {
	Ports           PortConfig
	IPFSRecovery    *IPFSRecoveryConfig
	TrustedNetwork  bool
	BootStrap       []string
	TestBootStrap   []string
	AsyncFTResponse bool
	UnpledgeConfig  UnpledgePoolConfig
	NodeConfigDir   string
}

type RubixConfig struct {
	// CfgData provides the nested .CfgData.Ports / .CfgData.IPFSRecovery access path.
	CfgData       RubixCfgData
	NodeConfigDir string
	NodeDir       string
	// A directory under NodeDir to store IPFS DID
	// and other files such as SC and NFT
	NetworkDir              string
	DidDir                  string
	NFTDir                  string
	SmartContractDir        string
	PortConfig              PortConfig
	UnpledgePoolConfig      UnpledgePoolConfig
	IPFSRecoveryConfig      IPFSRecoveryConfig
	DBConfig                DBConfig
	MainnetBootstrap        []string
	TestnetBootstrap        []string
	LocalnetBootStrap       []string
	EnableOptimizedUnpledge bool
	AsyncFTResponse         bool
	NodePort                int
	LogConfig               LogConfig
}
