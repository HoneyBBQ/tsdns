package cli

import (
	"github.com/honeybbq/tsdns/internal/config"
	"github.com/honeybbq/tsdns/internal/storage"
	"github.com/spf13/cobra"
)

func newMigrateCommand(configPath *string) *cobra.Command {
	var dsn string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply storage migrations (if applicable) and exit",
		RunE: func(_ *cobra.Command, _ []string) error {
			if dsn == "" {
				cfg, err := config.Load(*configPath)
				if err != nil {
					return err
				}
				dsn = cfg.Storage.DSN
			}

			if dsn == "" {
				return errStorageRequired
			}

			// For SQL backends, AutoMigrate runs when opening the repository.
			// For Redis, this command acts as a connectivity check.
			repo, _, err := storage.Open(dsn)
			if err != nil {
				return err
			}

			return repo.Close()
		},
	}

	cmd.Flags().StringVar(&dsn, "dsn", "", "Storage DSN (overrides config)")

	return cmd
}
