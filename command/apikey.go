package command

import (
	"fmt"
	"os"
	"time"

	"github.com/rubixchain/rubixgoplatform/server"
)

// ensureAPIKey creates the node API key on first run; it returns false only when startup must abort.
func (cmd *Command) ensureAPIKey(path string) bool {
	rec, err := server.LoadAPIKey(path)
	if os.IsNotExist(err) {
		rec, err = server.GenerateAPIKey(path, cmd.apiKeyValidity)
		if err != nil {
			cmd.log.Error("failed to create node API key", "err", err)
			return false
		}
		cmd.log.Info("Created node API key", "path", path, "expires", formatAPIKeyExpiry(rec))
		cmd.log.Info(fmt.Sprintf("Run '%s -p %s' to view it", GetAPIKeyCmd, cmd.nodeConfigPath))
		return true
	}
	if err != nil {
		cmd.log.Error("failed to load node API key", "err", err)
		return false
	}
	if status := rec.Status(time.Now()); status != "active" {
		cmd.log.Warn(fmt.Sprintf("Node API key is %s; transaction, signature, FT, NFT and smart contract APIs are disabled until '%s -p %s' is run", status, RegenerateAPIKeyCmd, cmd.nodeConfigPath))
		return true
	}
	cmd.log.Info("Using node API key", "path", path, "expires", formatAPIKeyExpiry(rec))
	return true
}

// setClientAPIKey attaches -apiKey, or else the active key from the -p dir, to CLI requests.
func (cmd *Command) setClientAPIKey() {
	if cmd.apiKey != "" {
		cmd.c.SetAPIKey(cmd.apiKey)
		return
	}
	rec, err := server.LoadAPIKey(server.APIKeyPath(cmd.nodeConfigPath))
	if err != nil || rec.Status(time.Now()) != "active" {
		return
	}
	cmd.c.SetAPIKey(rec.Key)
}

func (cmd *Command) getAPIKey() {
	path := server.APIKeyPath(cmd.nodeConfigPath)
	rec, err := server.LoadAPIKey(path)
	if os.IsNotExist(err) {
		fmt.Printf("No API key found at %s; run the node once to create it, or run '%s'\n", path, RegenerateAPIKeyCmd)
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	printAPIKey(rec)
}

func (cmd *Command) regenerateAPIKey() {
	rec, err := server.GenerateAPIKey(server.APIKeyPath(cmd.nodeConfigPath), cmd.apiKeyValidity)
	if err != nil {
		fmt.Println("Failed to regenerate API key:", err)
		return
	}
	fmt.Println("New API key generated; the previous key no longer works")
	printAPIKey(rec)
}

func (cmd *Command) revokeAPIKey() {
	path := server.APIKeyPath(cmd.nodeConfigPath)
	if err := server.RevokeAPIKey(path); err != nil {
		fmt.Println("Failed to revoke API key:", err)
		return
	}
	fmt.Printf("API key revoked; protected APIs are disabled until '%s -p %s' is run\n", RegenerateAPIKeyCmd, cmd.nodeConfigPath)
}

func printAPIKey(rec *server.APIKeyRecord) {
	key := rec.Key
	if key == "" {
		key = "-"
	}
	fmt.Println("API key    :", key)
	fmt.Println("Status     :", rec.Status(time.Now()))
	fmt.Println("Created at :", rec.CreatedAt.Format(time.RFC3339))
	fmt.Println("Expires at :", formatAPIKeyExpiry(rec))
}

func formatAPIKeyExpiry(rec *server.APIKeyRecord) string {
	if rec.ExpiresAt.IsZero() {
		return "unlimited"
	}
	return rec.ExpiresAt.Format(time.RFC3339)
}
