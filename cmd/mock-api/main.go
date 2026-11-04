// Command mock-api serves reloadable Great Firewall profiles over authenticated HTTPS.
package main

import (
	"log/slog"
	"os"

	"github.com/greatfirewall/operator/internal/cli"
	ctrl "sigs.k8s.io/controller-runtime"
)

// main runs the mock API until a termination signal and reports startup failures.
func main() {
	if err := cli.NewCommand(true).ExecuteContext(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("Mock API failed", "error", err)
		os.Exit(1)
	}
}
