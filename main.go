package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	cliFlagDBDSN    = "db-dsn"
	cliFlagLogLevel = "log-level"
	cliFlagLogDevel = "log-devel"
)

func bindDBDSNFlag(fs *pflag.FlagSet) {
	fs.String(cliFlagDBDSN, "", "Database data source name to use.")
}

func createMainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "sqlite-rest",
		Short:        "Serve a RESTful API from a SQLite database",
		SilenceUsage: true,
	}

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
