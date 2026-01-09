package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Execute runs the root command and handles any execution errors.
func Execute() {
	err := NewRootCommand().Execute()
	if err != nil {
		os.Exit(1)
	}
}

// NewRootCommand creates the main 'tsdns' command.
func NewRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "tsdns",
		Short: "A TeamSpeak TSDNS compatible server",
	}

	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to config file (YAML)")

	root.AddCommand(
		newServeCommand(&configPath),
		newMigrateCommand(&configPath),
		newRecordsCommand(&configPath),
		newImportCommand(&configPath),
		newVersionCommand(),
	)

	return root
}

func buildVersion() string {
	return fmt.Sprintf("%s (commit=%s, date=%s)", version, commit, date)
}
