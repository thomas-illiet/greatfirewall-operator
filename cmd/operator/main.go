// Command operator synchronizes Great Firewall profiles with Kubernetes NetworkPolicies.
package main

import (
	"log/slog"
	"os"

	"github.com/greatfirewall/operator/internal/cli"
	ctrl "sigs.k8s.io/controller-runtime"
)

// main configures structured logging and runs the operator until a termination signal.
func main() {
	if err := cli.NewCommand(false).ExecuteContext(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("Operator failed", "error", err)
		os.Exit(1)
	}
}
