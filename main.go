package main

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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

func createServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "serve",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dsn, err := cmd.Flags().GetString(cliFlagDBDSN)
			if err != nil {
				return err
			}

			db, err := sqlx.Open("sqlite3", dsn)
			if err != nil {
				return err
			}
			defer db.Close()

			var logger logr.Logger
			{
				zapLog, err := zap.NewDevelopment()
				if err != nil {
					return err
				}
				logger = zapr.NewLogger(zapLog)
			}

			opts := &ServerOptions{
				Logger:  logger,
				Queryer: db,
				Execer:  db,
			}

			server, err := NewServer(opts)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server.Start(ctx.Done())

			return nil
		},
	}

	return cmd
}

func main() {
	cmd := createMainCmd()

	if cmd.Execute() != nil {
		os.Exit(1)
	}
}
