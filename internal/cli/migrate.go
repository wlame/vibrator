package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/migrate"
)

// migratePinFlags holds the flag state for the migrate-pin command.
type migratePinFlags struct {
	input   string
	output  string
	dryRun  bool
	keepOld bool
}

var migratePinFlagsState migratePinFlags

// migratePinCmd converts a bash-era `.vb.env` file (dotenv) into the
// new Go-era `.vb` (TOML) format. A one-shot helper for users coming
// from the bash version of vibrator.
//
// Default: read `.vb.env`, write `.vb`, archive the original to
// `.vb.env.bak`. Use --dry-run to preview without touching disk.
var migratePinCmd = &cobra.Command{
	Use:   "migrate-pin",
	Short: "Convert a legacy bash-era .vb.env into the new TOML .vb",
	Long: `Reads a workspace's .vb.env (key=value dotenv file from the bash version
of vibrator) and writes an equivalent .vb (TOML).

Known keys are mapped to structured fields:

  HARNESS / PROFILE / SHELL                    → top-level scalars
  WITH / NO / CATALOG                          → top-level lists
  CLAUDE_MEM_SERVER_BETA_*                     → [prereqs.claude-mem-server-beta]
  USERNAME                                     → (dropped — now a build-time flag)

Unrecognized keys are preserved under [env] verbatim, so a user's
OPENAI_API_KEY or custom shell-sourced var carries over.

The original .vb.env is moved to .vb.env.bak unless --keep-old is set.`,
	RunE: runMigratePin,
}

func init() {
	migratePinCmd.Flags().StringVar(&migratePinFlagsState.input, "input", "",
		"Path to .vb.env (default: ./vb.env in the current dir).")
	migratePinCmd.Flags().StringVar(&migratePinFlagsState.output, "output", "",
		"Path to write .vb (default: .vb in the same dir as the input).")
	migratePinCmd.Flags().BoolVar(&migratePinFlagsState.dryRun, "dry-run", false,
		"Print the conversion plan and resulting TOML without touching disk.")
	migratePinCmd.Flags().BoolVar(&migratePinFlagsState.keepOld, "keep-old", false,
		"Keep .vb.env in place (default: move to .vb.env.bak after success).")

	rootCmd.AddCommand(migratePinCmd)
}

func runMigratePin(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()

	// Resolve input path.
	inputPath := migratePinFlagsState.input
	if inputPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		inputPath = filepath.Join(cwd, ".vb.env")
	}
	if _, err := os.Stat(inputPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no .vb.env found at %s (use --input=<path> to specify)", inputPath)
		}
		return err
	}

	// Resolve output path.
	outputPath := migratePinFlagsState.output
	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(inputPath), config.PinFileName)
	}

	// Parse the dotenv.
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", inputPath, err)
	}
	defer f.Close()

	kvs, err := migrate.ParseDotenv(f)
	if err != nil {
		return err
	}
	pin, notes := migrate.ToPin(kvs)

	// Print the conversion log.
	fmt.Fprintf(stderr, "→ Parsing %s (%d keys)\n", inputPath, len(kvs))
	for _, n := range notes {
		fmt.Fprintf(stderr, "  %s\n", n)
	}

	if migratePinFlagsState.dryRun {
		// In dry-run, emit the TOML to stdout without touching disk.
		fmt.Fprintf(stderr, "\n%s(dry-run — not writing %s)%s\n", c.dim, outputPath, c.reset)
		fmt.Fprintln(stdout)
		// Use a temp path to leverage Save's encoder, then read+print.
		tmp, err := os.CreateTemp("", "vb-migrate-*.toml")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		tmp.Close()
		defer os.Remove(tmpPath)
		if err := config.Save(tmpPath, &pin); err != nil {
			return err
		}
		body, err := os.ReadFile(tmpPath)
		if err != nil {
			return err
		}
		_, _ = stdout.Write(body)
		return nil
	}

	// Refuse to clobber an existing .vb that isn't empty.
	if existing, err := os.Stat(outputPath); err == nil && existing.Size() > 0 {
		return fmt.Errorf("%s already exists and is non-empty (remove it first or use --output=<path>)", outputPath)
	}

	if err := config.Save(outputPath, &pin); err != nil {
		return fmt.Errorf("save %s: %w", outputPath, err)
	}
	fmt.Fprintf(stderr, "\n%s✓ wrote %s%s\n", c.green, outputPath, c.reset)

	// Update .gitignore if applicable.
	wsDir := filepath.Dir(outputPath)
	if added, err := config.AppendToGitignore(wsDir); err == nil && added {
		fmt.Fprintf(stderr, "%s✓ added .vb to %s/.gitignore%s\n", c.green, wsDir, c.reset)
	}

	// Archive the original.
	if !migratePinFlagsState.keepOld {
		bakPath := inputPath + ".bak"
		if err := os.Rename(inputPath, bakPath); err != nil {
			fmt.Fprintf(stderr, "%swarning: could not archive original to %s: %v%s\n",
				c.yellow, bakPath, err, c.reset)
		} else {
			fmt.Fprintf(stderr, "%s✓ archived original to %s%s\n", c.green, bakPath, c.reset)
		}
	}
	return nil
}
