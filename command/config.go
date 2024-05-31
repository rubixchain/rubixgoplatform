package command

import (
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/spf13/cobra"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

func configCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Config related subcommands (DB, service)",
		Long:  "Config related subcommands (DB, service)",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		setupDB(cmdCfg),
		setupService(cmdCfg),
	)

	return cmd 
}

func setupDB(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "setup-db",
		Short: "Setup Database",
		Long: "Setup Database",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sc := &config.StorageConfig{
				StorageType: cmdCfg.storageType,
				DBName:      cmdCfg.dbName,
				DBAddress:   cmdCfg.dbAddress,
				DBUserName:  cmdCfg.dbUserName,
				DBPassword:  cmdCfg.dbPassword,
				DBPort:      cmdCfg.dbPort,
				DBType:      cmdCfg.dbType,
			}
			msg, ok := cmdCfg.c.SetupDB(sc)
			if !ok {
				cmdCfg.log.Error("Failed to setup DB", "msg", msg)
				return nil
			}
			cmdCfg.log.Info("DB setup done successfully")
			return nil
		},
	}

	cmd.Flags().IntVar(&cmdCfg.storageType, "storageType", storage.StorageDBType, "Storage type")
	cmd.Flags().StringVar(&cmdCfg.dbName, "dbName", "ServiceDB", "Service database name")
	cmd.Flags().StringVar(&cmdCfg.dbType, "dbType", "SQLServer", "DB Type, supported database are SQLServer, PostgressSQL, MySQL & Sqlite3")
	cmd.Flags().StringVar(&cmdCfg.dbAddress, "dbAddress", "localhost", "Database address")
	cmd.Flags().StringVar(&cmdCfg.dbPort, "dbPort", "1433", "Database port number")
	cmd.Flags().StringVar(&cmdCfg.dbUserName, "dbUsername", "sa", "Database username")
	cmd.Flags().StringVar(&cmdCfg.dbPassword, "dbPassword", "password", "Database password")

	return cmd
}

func setupService(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "setup-service",
		Short: "Setup Service",
		Long: "Setup Service",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scfg := config.ServiceConfig{
				ServiceName: cmdCfg.srvName,
				DBName:      cmdCfg.dbName,
				DBType:      cmdCfg.dbType,
				DBAddress:   cmdCfg.dbAddress,
				DBPort:      cmdCfg.dbPort,
				DBUserName:  cmdCfg.dbUserName,
				DBPassword:  cmdCfg.dbPassword,
			}
			msg, status := cmdCfg.c.SetupService(&scfg)
			if !status {
				cmdCfg.log.Error("Failed to setup service", "message", msg)
				return nil
			}

			cmdCfg.log.Info("Service setup successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.srvName, "srvName", "explorer_service", "Service name")
	cmd.Flags().StringVar(&cmdCfg.dbName, "dbName", "ServiceDB", "Service database name")
	cmd.Flags().StringVar(&cmdCfg.dbType, "dbType", "SQLServer", "DB Type, supported database are SQLServer, PostgressSQL, MySQL & Sqlite3")
	cmd.Flags().StringVar(&cmdCfg.dbAddress, "dbAddress", "localhost", "Database address")
	cmd.Flags().StringVar(&cmdCfg.dbPort, "dbPort", "1433", "Database port number")
	cmd.Flags().StringVar(&cmdCfg.dbUserName, "dbUsername", "sa", "Database username")
	cmd.Flags().StringVar(&cmdCfg.dbPassword, "dbPassword", "password", "Database password")

	return cmd
}