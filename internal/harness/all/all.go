// Package all is a side-effect-only import that registers every built-in
// harness with the global harness.Registry.
//
// The vibrate binary imports this package (blank-imported) once from
// cmd/vibrate/main.go so harness.ByID() and the catalog CLI work.
// Subpackages that already pull in specific harnesses (e.g., tests
// targeting one harness) can import only what they need.
package all

import (
	_ "github.com/wlame/vibrator/internal/harness/claudecode"
	_ "github.com/wlame/vibrator/internal/harness/codex"
	_ "github.com/wlame/vibrator/internal/harness/opencode"
	_ "github.com/wlame/vibrator/internal/harness/pi"
)
