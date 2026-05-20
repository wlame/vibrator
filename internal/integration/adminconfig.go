package integration

// AdminConfigSchema declares the host-side configuration an integration
// uses.
//
// For step 1 this carries only the file path — the actual form fields
// and validation live in each integration's CLI driver (e.g.,
// internal/cli/integrations_claudemem.go). A later step will generalize
// form rendering from a declarative field schema so simple integrations
// can describe their admin form purely in data.
type AdminConfigSchema struct {
	// Path is the absolute path to the admin config file (typically
	// under ~/.config/vibrator/<id>.toml). Used by the CLI to display
	// where state is stored, even before the file exists.
	Path string
}
