package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/cobra"
)

const tableNameMigrations = "__sqlite_rest_migrations"

func createMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "migrate migrations-dir",
		Short:        "Apply database migrations",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, err := createLogger(cmd)
			if err != nil {
				setupLogger.Error(err, "failed to create logger")
				return err
			}

			db, err := openDB(cmd)
			if err != nil {
				setupLogger.Error(err, "create db")
				return err
			}
			defer db.Close()

			opts := &MigrateOptions{
				Logger:    logger,
				DB:        db.DB,
				SourceDIR: args[0],
			}
			migrator, err := NewMigrator(opts)
			if err != nil {
				setupLogger.Error(err, "failed to create migrator")
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := migrator.Up(ctx); err != nil {
				return err
			}

			return nil
		},
	}

	return cmd
}

type MigrateOptions struct {
	Logger    logr.Logger
	DB        *sql.DB
	SourceDIR string
}

func (opts *MigrateOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	if opts.DB == nil {
		return fmt.Errorf(".DB is required")
	}

	if opts.SourceDIR == "" {
		return fmt.Errorf(".SourceDIR is required")
	}
	if s, err := filepath.Abs(opts.SourceDIR); err == nil {
		opts.SourceDIR = s
	} else {
		return fmt.Errorf("failed to resolve SourceDIR %q: %w", opts.SourceDIR, err)
	}
	stat, err := os.Stat(opts.SourceDIR)
	if err != nil {
		return fmt.Errorf("%s: %w", opts.SourceDIR, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("migrations source dir %q is not a dir", opts.SourceDIR)
	}

	return nil
}

type dbMigrator struct {
	logger   logr.Logger
	migrator *migrate.Migrate
}

func NewMigrator(opts *MigrateOptions) (*dbMigrator, error) {
	if err := opts.defaults(); err != nil {
		return nil, err
	}

	driver, err := sqlite3.WithInstance(opts.DB, &sqlite3.Config{
		MigrationsTable: tableNameMigrations,
	})
	if err != nil {
		return nil, err
	}
	migrator, err := migrate.NewWithDatabaseInstance(
		"file://"+opts.SourceDIR,
		"sqlite3", driver,
	)
	if err != nil {
		return nil, err
	}

	rv := &dbMigrator{
		logger:   opts.Logger.WithName("db-migrator"),
		migrator: migrator,
	}

	return rv, nil
}

func (m *dbMigrator) Up(ctx context.Context) error {
	logger := m.logger.WithName("up")
	logger.Info("applying operation")

	err := m.migrator.Up()
	if err == nil {
		logger.Info("applied operation")

		return nil
	}

	if errors.Is(err, migrate.ErrNoChange) {
		// no update
		logger.V(8).Info("no pending migrations")
		return nil
	}

	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		// no migrations set
		if pathErr.Op == "first" && errors.Is(pathErr.Err, fs.ErrNotExist) {
			logger.Info("no migrations to apply")
			return nil
		}
	}

	logger.Error(err, "failed to apply operation")
	return fmt.Errorf("up: %w", err)
}
