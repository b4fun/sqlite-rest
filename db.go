package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

func openDB(cmd *cobra.Command) (*sqlx.DB, error) {
	dsn, err := cmd.Flags().GetString(cliFlagDBDSN)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", cliFlagDBDSN, err)
	}

	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	return db, nil
}
