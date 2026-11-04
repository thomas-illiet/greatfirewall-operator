// Package cli provides Cobra entry points and a shared slog logging configuration.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"github.com/greatfirewall/operator/internal/config"
	"github.com/greatfirewall/operator/internal/mockserver"
	"github.com/greatfirewall/operator/internal/operator"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NewCommand constructs the production operator or mock command with mandatory configuration.
func NewCommand(mock bool) *cobra.Command {
	return command(mock, operator.Start, mockserver.Start, os.Stderr)
}

// command keeps command parsing independent of service startup for deterministic CLI tests.
func command(
	mock bool,
	startOperator func(context.Context, config.Operator) error,
	startMock func(context.Context, config.Mock) error,
	logs io.Writer,
) *cobra.Command {
	var path, level string
	name := "greatfirewall-operator"
	if mock {
		name = "greatfirewall-mock-api"
	}
	root := &cobra.Command{
		Use:           name,
		Short:         "IPv4-only Great Firewall network isolation",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
	}
	root.Flags().StringVar(&path, "config", "", "Path to YAML configuration")
	root.Flags().StringVar(&level, "log-level", "info", "Log level: debug, info, warn, error")
	_ = root.MarkFlagRequired("config")
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := configureLogging(logs, level); err != nil {
			return err
		}
		if mock {
			settings, err := config.LoadMock(path)
			if err != nil {
				return err
			}
			return startMock(cmd.Context(), settings)
		}
		settings, err := config.LoadOperator(path)
		if err != nil {
			return err
		}
		return startOperator(cmd.Context(), settings)
	}
	return root
}

// configureLogging routes application and controller-runtime logs through one JSON slog handler.
func configureLogging(output io.Writer, value string) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return fmt.Errorf("invalid log level")
	}
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
	ctrl.SetLogger(logr.FromSlogHandler(handler))
	return nil
}
