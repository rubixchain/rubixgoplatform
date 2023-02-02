package config

import "github.com/rubixchain/rubixgoplatform/core/wallet"

// Ports defines varies ports used
type Ports struct {
	SendPort     uint16 `json:"sender_port"`
	ReceiverPort uint16 `json:"receiver_port"`
	IPFSPort     uint16 `json:"ipfs_port"`
	SwarmPort    uint16 `json:"swarm_port"`
	IPFSAPIPort  uint16 `json:"ipfs_api_port"`
}

type DIDConfigType struct {
	Type   int                    `json:"TYPE"`
	Config map[string]interface{} `json:"CONFIG"`
}

// ConfigData defines configuration data
type ConfigData struct {
	Ports            Ports               `json:"ports"`
	BootStrap        []string            `json:"bootstrap"`
	Services         map[string]string   `json:"services"`
	QuorumList       QuorumList          `json:"quorumr_list"`
	MainWalletConfig wallet.WalletConfig `json:"main_wallet_config"`
	TestWalletConfig wallet.WalletConfig `json:"test_wallet_config"`
}

type QuorumList struct {
	Alpha []string `json:"alpha"`
	Beta  []string `json:"beta"`
	Gamma []string `json:"gamma"`
}

type Config struct {
	NodeAddress string     `json:"node_address"`
	NodePort    string     `json:"node_port"`
	DirPath     string     `json:"dir_path"`
	CfgData     ConfigData `json:"cfg_data"`
}

type ServiceConfig struct {
	ServiceSettings string `json:"service_settings"` // ServiceSettings settings for the service
	ServiceName     string `json:"service_name"`     // ServiceName name of the service
	DBName          string `json:"db_name"`          // DBName is the name of the db.
	DBAddress       string `json:"db_address"`       // DBPath is the name of the database itself.
	DBPort          string `json:"db_port"`          // DBPath is the name of the database itself.
	DBType          string `json:"db_type"`          // DBType is type of database to use
	DBUserName      string `json:"db_user_name"`     // DBUserName is the user name for the DB
	DBPassword      string `json:"db_password"`      // DBPassword is the password  for the user

}
