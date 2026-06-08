package app

import (
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/workspace"
)

// PinContainerName computes the docker container name vibrate would assign
// to the given pin at the given workspace directory. This uses the same
// fingerprint + naming logic as Run, so the returned name matches the
// container that was created (or would be created) for that pin.
//
// Useful when you need to reference the old container before overwriting
// the pin — e.g., `vibrate reconfigure` needs the old name before running
// the wizard that produces a new spec with a different fingerprint.
func PinContainerName(pin config.Pin, wsDir string) (string, error) {
	_, wsSpec, err := buildSpecs(pin, Options{})
	if err != nil {
		return "", err
	}
	fp := workspace.Fingerprint(wsSpec)
	return workspace.ContainerName(wsDir, fp), nil
}
