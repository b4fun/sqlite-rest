package main

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var setupLogger logr.Logger = logr.Discard()

func init() {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.Level = zap.NewAtomicLevelAt(zapcore.Level(-12))
	zapLog, err := zapConfig.Build()
	if err != nil {
		panic(err)
	}

	setupLogger = zapr.NewLogger(zapLog).WithName("setup")
}

func createLogger(cmd *cobra.Command) (logr.Logger, error) {
	logLevel, err := cmd.Flags().GetInt8(cliFlagLogLevel)
	if err != nil {
		return logr.Discard(), fmt.Errorf("read %s: %w", cliFlagLogLevel, err)
	}
	logDevel, err := cmd.Flags().GetBool(cliFlagLogDevel)
	if err != nil {
		return logr.Discard(), fmt.Errorf("read %s: %w", cliFlagLogDevel, err)
	}

	var zapConfig zap.Config
	if logDevel {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}
	zapConfig.Level = zap.NewAtomicLevelAt(zapcore.Level(-logLevel))
	zapLog, err := zapConfig.Build()
	if err != nil {
		return logr.Discard(), fmt.Errorf("create logger: %w", err)
	}

	return zapr.NewLogger(zapLog), nil
}
