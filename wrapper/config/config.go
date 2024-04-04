package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

// Config represents the configuration information.
type Config struct {
	DBName        string `json:"db_name"`        // DBName is the name of the db.
	DBAddress     string `json:"db_address"`     // DBPath is the name of the database itself.
	DBPort        string `json:"db_port"`        // DBPath is the name of the database itself.
	DBType        string `json:"db_type"`        // DBType is type of database to use
	DBUserName    string `json:"db_user_name"`   // DBUserName is the user name for the DB
	DBPassword    string `json:"db_password"`    // DBPassword is the password  for the user
	HostAddress   string `json:"host_address"`   // HostAddress is the address to listen for connections on.
	HostPort      string `json:"host_port"`      // HostPort is the port to listen on.
	ServerAddress string `json:"server_address"` // HostAddress is the address to listen for connections on.
	ServerPort    string `json:"server_port"`    // HostPort is the port to listen on.
	LogFile       string `json:"log_file"`       // LogFile is an optional file to log messages to.
	Production    string `json:"production"`     // Production flag
	CertFile      string `json:"cert_file"`      // Certificate file
	KeyFile       string `json:"key_file"`       // Key file
	ClientID      string `json:"client_id"`      // Client ID
	ClientSecret  string `json:"client_secret"`  // Client Secret
	TenantName    string `json:"tenant_name"`    // Tenant name
	TokenSecret   string `json:"token_secret"`   // Token Secret
	PatchPath     string `json:"patch_path"`     // Patch Path
	LogLevel      int    `json:"log_level"`      // log level
}

// LoadConfig loads a configuration at the provided filepath, returning the
// parsed configuration.
func LoadConfig(filepath string) (*Config, error) {
	// Get the config file
	configFile, err := ioutil.ReadFile(filepath)
	if err != nil {
		fmt.Printf("File error: %v\n", err)
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(configFile, config)
	return config, err
}
