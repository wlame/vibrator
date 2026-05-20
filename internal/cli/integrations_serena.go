package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/integration"
)

// runIntegrationsSerena dispatches the Serena flow through the generic
// runner. The registered descriptor (see
// internal/integration/serena/descriptor.go) carries everything the
// runner needs: runtimes, probe, wiring.
func runIntegrationsSerena(cmd *cobra.Command, _ []string) error {
	integ, ok := integration.Get("serena")
	if !ok {
		return fmt.Errorf("serena integration not registered (build error?)")
	}
	return runIntegration(cmd, integ)
}
