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
	SendPort           uint16 `json:"SEND_PORT"`
	ReceiverPort       uint16 `json:"RECEIVER_PORT"`
	SellerPort         uint16 `json:"SELLER_PORT"`
	BuyerPort          uint16 `json:"BUYER_PORT"`
	GossipReceiverPort uint16 `json:"GOSSIP_RECEIVER"`
	GossipSenderPort   uint16 `json:"GOSSIP_SENDER"`
	QuorumPort         uint16 `json:"QUORUM_PORT"`
	Sender2Q1Port      uint16 `json:"SENDER2Q1"`
	Sender2Q2Port      uint16 `json:"SENDER2Q2"`
	Sender2Q3Port      uint16 `json:"SENDER2Q3"`
	Sender2Q4Port      uint16 `json:"SENDER2Q4"`
	Sender2Q5Port      uint16 `json:"SENDER2Q5"`
	Sender2Q6Port      uint16 `json:"SENDER2Q6"`
	Sender2Q7Port      uint16 `json:"SENDER2Q7"`
	IPFSPort           uint16 `json:"IPFS_PORT"`
	SwarmPort          uint16 `json:"SWARM_PORT"`
	IPFSAPIPort        uint16 `json:"IPFS_API_PORT"`
	AppPort            uint16 `json:"APPLICATION_PORT"`
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
	Paths         Paths                    `json:"PATHS"`
	Ports         Ports                    `json:"PORTS"`
	SyncConfig    SyncConfig               `json:"SYNCCONFIG"`
	ConsensusData ConsensusData            `json:"CONSENSUS"`
	QuorumList    QuorumList               `json:"QUORUM_LIST"`
	DIDList       []string                 `json:"DID_LIST"`
	DIDConfig     map[string]DIDConfigType `json:"DID_CONFIG"`
	BootStrap     []string                 `json:"BOOTSTRAP"`
}

type Config struct {
	NodeAddress string
	NodePort    string
	DirPath     string
	CfgData     ConfigData
}
