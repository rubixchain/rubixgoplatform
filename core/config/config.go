package config

// Paths defines varies path
type Paths struct {
	LogPath           string `json:"LOGGER_PATH"`
	DataPath          string `json:"DATA_PATH"`
	TokensPath        string `json:"TOKENS_PATH"`
	TokenChainPath    string `json:"TOKENCHAIN_PATH"`
	NFTTokensPath     string `json:"NFT_TOKENS_PATH"`
	NFTTokenChainPath string `json:"NFT_TOKENCHAIN_PATH"`
	NFTTHidePath      string `json:"NFT_HIDE_PATH"`
	WalletDataPath    string `json:"WALLET_DATA_PATH"`
	PaymentsPath      string `json:"PAYMENTS_PATH"`
}

// Ports defines varies ports used
type Ports struct {
	SendPort     uint16 `json:"sender_port"`
	ReceiverPort uint16 `json:"receiver_port"`
	IPFSPort     uint16 `json:"ipfs_port"`
	SwarmPort    uint16 `json:"swarm_port"`
	IPFSAPIPort  uint16 `json:"ipfs_api_port"`
}

// SyncConfig defines varies IPs
type SyncConfig struct {
	SyncIP        string `json:"SYNC_IP"`
	ExplorerIP    string `json:"EXPLORER_IP"`
	NFTExplorerIP string `json:"NFT_EXPLORER_IP"`
	UserDIDIP     string `json:"USERDID_IP"`
	AdvisoryIP    string `json:"ADVISORY_IP"`
}

// ConsensusData defines consensus data
type ConsensusData struct {
	ConsensusStatus bool `json:"CONSENSUS_STATUS"`
	QuorumCount     int  `json:"QUORUM_COUNT"`
}

// QuorumList defines quorum list
type QuorumList struct {
	Quorum1 string `json:"QUORUM_1"`
	Quorum2 string `json:"QUORUM_2"`
	Quorum3 string `json:"QUORUM_3"`
	Quorum4 string `json:"QUORUM_4"`
	Quorum5 string `json:"QUORUM_5"`
	Quorum6 string `json:"QUORUM_6"`
	Quorum7 string `json:"QUORUM_7"`
}

type DIDConfigType struct {
	Type   int                    `json:"TYPE"`
	Config map[string]interface{} `json:"CONFIG"`
}

// ConfigData defines configuration data
type ConfigData struct {
	Ports     Ports                    `json:"ports"`
	DIDList   []string                 `json:"did_list"`
	DIDConfig map[string]DIDConfigType `json:"did_config"`
	BootStrap []string                 `json:"bootstrap"`
}

type Config struct {
	NodeAddress string     `json:"node_address"`
	NodePort    string     `json:"node_port"`
	DirPath     string     `json:"dir_path"`
	CfgData     ConfigData `json:"cfg_data"`
}
