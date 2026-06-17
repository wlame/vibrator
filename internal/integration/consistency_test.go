package integration_test

import (
	"testing"

	"github.com/wlame/vibrator/internal/harness"
	_ "github.com/wlame/vibrator/internal/harness/all"
	"github.com/wlame/vibrator/internal/integration"
	_ "github.com/wlame/vibrator/internal/integration/claudemem"
	_ "github.com/wlame/vibrator/internal/integration/serena"
)

// Wiring entries are filtered against harness registry IDs at manifest
// build time (BuildManifest); an unknown ID silently drops the wiring,
// which disabled the whole integrations manifest once already. Guard the
// invariant: every registered descriptor targets a registered harness.
func TestWiringHarnessIDsAreRegistered(t *testing.T) {
	for _, integ := range integration.All() {
		for _, w := range integ.Wiring {
			if w.Harness == "*" {
				continue
			}
			if _, ok := harness.ByID(w.Harness); !ok {
				t.Errorf("integration %q wiring targets unknown harness %q (registered: %v)",
					integ.ID, w.Harness, harness.IDs())
			}
		}
	}
}
