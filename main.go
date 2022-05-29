package main

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	cliFlagDBDSN    = "db-dsn"
	cliFlagLogLevel = "log-level"
	cliFlagLogDevel = "log-devel"
)

func createMainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "sqlite-rest",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String(cliFlagDBDSN, "", "Database data source name to use.")
	cmd.PersistentFlags().
		Int8(cliFlagLogLevel, 5, "Log level to use. Use 8 or more for verbose log.")
	cmd.PersistentFlags().
		Bool(cliFlagLogDevel, false, "Enable devel log format?")

	cmd.AddCommand(
		createServeCmd(),
		createMigrateCmd(),
	)

	cmd.CompletionOptions.DisableDefaultCmd = true

	return cmd
}

func main() {
	cmd := createMainCmd()

	if cmd.Execute() != nil {
		os.Exit(1)
	}
}
