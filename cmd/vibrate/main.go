// Command vibrate runs AI coding agents in dockerized isolation per workspace.
//
// See the project README for setup and usage. cmd/vibrate is intentionally
// tiny — all logic lives in internal/ packages so it can be unit-tested
// without going through main().
package main

import (
	"fmt"
	"os"

	"github.com/wlame/vibrator/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Errors are already printed via cobra's SetErr / log helpers;
		// just propagate the failure exit code.
		fmt.Fprintln(os.Stderr, "vibrate:", err)
		os.Exit(1)
	}
}
