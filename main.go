package main

import (
	"os"

	"github.com/spf13/cobra"
)

const cliFlagDBDSN = "db-dsn"

func createMainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "sqlite-rest",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String(cliFlagDBDSN, "", "Database data source name to use.")

	cmd.AddCommand(
		createServeCmd(),
	)

	return cmd
}

func main() {
	cmd := createMainCmd()

	if cmd.Execute() != nil {
		os.Exit(1)
	}
}
