package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/apiconfig"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}
	
	mode := os.Args[1]
	
	switch mode {
	case "decrypt":
		if len(os.Args) < 4 {
			fmt.Println("Error: Missing arguments for decrypt")
			printUsage()
			return
		}
		decrypt(os.Args[2], os.Args[3])
		
	case "encrypt":
		if len(os.Args) < 4 {
			fmt.Println("Error: Missing arguments for encrypt")
			printUsage()
			return
		}
		encrypt(os.Args[2], os.Args[3])
		
	case "enable-trusted":
		if len(os.Args) < 3 {
			fmt.Println("Error: Missing encryption key")
			printUsage()
			return
		}
		enableTrustedMode(os.Args[2])
		
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Rubix Config Converter - Work with plain JSON configs")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  Decrypt config to JSON:")
	fmt.Println("    go run config_converter.go decrypt <encrypted_config> <key>")
	fmt.Println()
	fmt.Println("  Encrypt JSON to config:")
	fmt.Println("    go run config_converter.go encrypt <plain.json> <key>")
	fmt.Println()
	fmt.Println("  Enable trusted network mode:")
	fmt.Println("    go run config_converter.go enable-trusted <key>")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run config_converter.go decrypt rubixconfig mypassword")
	fmt.Println("  go run config_converter.go encrypt config_plain.json mypassword")
	fmt.Println("  go run config_converter.go enable-trusted mypassword")
}

func decrypt(encFile, key string) {
	var cfg config.Config
	err := apiconfig.LoadAPIConfig(encFile, key, &cfg)
	if err != nil {
		fmt.Printf("Error loading encrypted config: %v\n", err)
		return
	}
	
	// Save as plain JSON with indentation
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling config: %v\n", err)
		return
	}
	
	outputFile := "config_plain.json"
	err = ioutil.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Config decrypted to %s\n", outputFile)
	fmt.Println("  You can now edit this file with any text editor")
	fmt.Printf("  To re-encrypt: go run config_converter.go encrypt %s %s\n", outputFile, key)
}

func encrypt(plainFile, key string) {
	// Read plain JSON
	data, err := ioutil.ReadFile(plainFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	
	// Verify it's valid JSON
	var cfg config.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		fmt.Printf("Error: Invalid JSON in %s: %v\n", plainFile, err)
		return
	}
	
	// Create encrypted config
	outputFile := "rubixconfig"
	err = apiconfig.CreateAPIConfig(outputFile, key, data)
	if err != nil {
		fmt.Printf("Error creating encrypted config: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Config encrypted to %s\n", outputFile)
	fmt.Println("  You can now use this with: ./rubixgoplatform run -p 20000")
}

func enableTrustedMode(key string) {
	// Try common config file names
	configFiles := []string{"rubixconfig", "node1.config", "node2.config"}
	var configFile string
	
	for _, cf := range configFiles {
		if _, err := os.Stat(cf); err == nil {
			configFile = cf
			break
		}
	}
	
	if configFile == "" {
		fmt.Println("Error: No config file found (tried: rubixconfig, node1.config, node2.config)")
		return
	}
	
	fmt.Printf("Found config file: %s\n", configFile)
	
	// Load existing config
	var cfg config.Config
	err := apiconfig.LoadAPIConfig(configFile, key, &cfg)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}
	
	// Enable trusted network mode
	cfg.CfgData.TrustedNetwork = true
	
	// Save back
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		fmt.Printf("Error marshaling config: %v\n", err)
		return
	}
	
	err = apiconfig.CreateAPIConfig(configFile, key, jsonData)
	if err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Trusted network mode ENABLED in %s\n", configFile)
	fmt.Println("  DHT/pin checks will be skipped for better performance")
	fmt.Println("  WARNING: Only use in trusted environments!")
}