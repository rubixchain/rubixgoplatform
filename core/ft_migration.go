package core

import (
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// MigrateFTTransactionTokens starts the FT transaction token migration
func (c *Core) MigrateFTTransactionTokens() (*wallet.FTTransactionMigrationStatus, error) {
	c.log.Info("Starting FT transaction token migration")
	return c.w.MigrateFTTransactionTokens()
}

// GetFTMigrationStatus retrieves the current or last migration status
func (c *Core) GetFTMigrationStatus() (*wallet.FTTransactionMigrationStatus, error) {
	return c.w.GetFTMigrationStatus()
}