package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greatfirewall/operator/internal/config"
)

// TestCommands exercises Cobra parsing without starting Kubernetes or binding a server.
func TestCommands(t *testing.T) {
	var logs bytes.Buffer
	called := false
	startOperator := func(context.Context, config.Operator) error {
		called = true
		return nil
	}
	startMock := func(context.Context, config.Mock) error {
		called = true
		return nil
	}
	for _, mock := range []bool{false, true} {
		text := "namespace: system\nipv6Disabled: true\n"
		if mock {
			text = "tokenFile: /token\nprofilesFile: /profiles\ncertificateFile: /cert\nkeyFile: /key\n"
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		root := command(mock, startOperator, startMock, &logs)
		root.SetArgs([]string{"--config", path, "--log-level", "debug"})
		called = false
		if err := root.ExecuteContext(context.Background()); err != nil || !called {
			t.Fatalf("command did not dispatch: %v", err)
		}
	}
}

// TestCommandFailures checks argument, configuration and startup failures are preserved.
func TestCommandFailures(t *testing.T) {
	for _, mock := range []bool{false, true} {
		for _, args := range [][]string{
			{},
			{"--config", "missing", "--log-level", "invalid"},
			{"--config", "missing"},
			{"unexpected"},
		} {
			root := NewCommand(mock)
			root.SetArgs(args)
			if err := root.Execute(); err == nil {
				t.Fatal("invalid command accepted")
			}
		}
	}

	var logs bytes.Buffer
	if err := configureLogging(&logs, "info"); err != nil {
		t.Fatal(err)
	}
	if err := configureLogging(&logs, "not-a-level"); err == nil {
		t.Fatal("invalid log level accepted")
	}
	root := command(false, nil, nil, &logs)
	root.SetArgs([]string{"--help"})
	root.SetOut(&logs)
	if err := root.Execute(); err != nil || !strings.Contains(logs.String(), "--config") {
		t.Fatal(errors.New("help does not document configuration"))
	}
}
