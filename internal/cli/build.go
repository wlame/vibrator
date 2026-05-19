package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/app"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/profile"
	"github.com/wlame/vibrator/internal/workspace"
)

// Shared flag state for the build commands. Each command reads them; flags
// are declared once on the parent so the build/build-dockerfile pair stays
// consistent.
type buildFlags struct {
	harnessID    string
	profileID    string
	shell        string
	with         []string
	no           []string
	extensionIDs   []string
	username     string
	out          string // build-dockerfile only
	noCache      bool   // build only
	tag          string // build only — overrides the workspace-fingerprinted tag
	hostUID      int
	hostGID      int
}

// flag defaults — match the wizard's "if the user picks nothing" answers.
const (
	defaultProfile = profile.IDFull
	defaultShell   = "zsh"
)

var (
	buildDockerfileFlags buildFlags
	buildFlagsState      buildFlags
)

// buildCmd builds the Docker image without launching a container.
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the Docker image for the resolved workspace spec (no run)",
	Long: `Resolves the workspace spec (flags > .vb pin > defaults) and runs
'docker build' with the generated Dockerfile. Does not start a container.

Use --no-cache to force a clean rebuild.`,
	RunE: runBuild,
}

// dockerfileCmd generates the Dockerfile that `build` WOULD produce, without
// invoking docker. Used for inspection, debugging, and CI.
var dockerfileCmd = &cobra.Command{
	Use:   "build-dockerfile",
	Short: "Generate the Dockerfile for the resolved spec without building",
	Long: `Runs the same generation pipeline as 'build' but writes the resulting
Dockerfile to a path (or stdout if --out=-) instead of invoking 'docker build'.

Byte-deterministic for a given input spec — useful for diffing changes,
golden tests, and CI inspection.`,
	RunE: runBuildDockerfile,
}

func init() {
	registerBuildFlags(buildCmd, &buildFlagsState, true)
	registerBuildFlags(dockerfileCmd, &buildDockerfileFlags, false)

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(dockerfileCmd)
}

// registerBuildFlags attaches the shared spec-resolution flags to a cobra
// command. The buildOnly flag enables `--no-cache` and `--tag`, which only
// apply to `vibrate build` (not `build-dockerfile`).
func registerBuildFlags(cmd *cobra.Command, flags *buildFlags, buildOnly bool) {
	cmd.Flags().StringVar(&flags.harnessID, "harness", "",
		"Agent harness to install (claude-code, codex, opencode, pi). Required.")
	cmd.Flags().StringVar(&flags.profileID, "profile", defaultProfile,
		"Base profile: minimal, backend, frontend, full.")
	cmd.Flags().StringVar(&flags.shell, "shell", defaultShell,
		"Default shell inside the container: bash, zsh, fish.")
	cmd.Flags().StringSliceVar(&flags.with, "with", nil,
		"Features to enable on top of the profile (repeatable, comma-separated).")
	cmd.Flags().StringSliceVar(&flags.no, "no", nil,
		"Features to disable on top of the profile (repeatable, comma-separated).")
	cmd.Flags().StringSliceVar(&flags.extensionIDs, "extensions", nil,
		"Extension IDs to install (comma-separated; from the chosen harness).")
	cmd.Flags().StringVar(&flags.username, "username", app.HostUsername(),
		"Unprivileged user created inside the container. Defaults to the host user's name (sanitized for Linux useradd).")
	cmd.Flags().IntVar(&flags.hostUID, "host-uid", os.Getuid(),
		"Host UID baked as ARG so mounted file permissions match the caller.")
	cmd.Flags().IntVar(&flags.hostGID, "host-gid", os.Getgid(),
		"Host GID baked as ARG so mounted file permissions match the caller.")

	if buildOnly {
		cmd.Flags().BoolVar(&flags.noCache, "no-cache", false,
			"Pass --no-cache to docker build (forces full rebuild).")
		cmd.Flags().StringVar(&flags.tag, "tag", "",
			"Override the image tag (default: vb-<harness>-<profile>-<fingerprint>:latest).")
	} else {
		cmd.Flags().StringVar(&flags.out, "out", "-",
			"Output path; use - for stdout.")
	}
}

// resolveSpec turns flag state into a fully-validated dockerfile.Spec plus
// the workspace.Spec used for image-tag fingerprinting. Shared between
// build and build-dockerfile.
func resolveSpec(f *buildFlags) (dockerfile.Spec, workspace.Spec, error) {
	if f.harnessID == "" {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("--harness is required (one of: %s)",
			strings.Join(harness.IDs(), ", "))
	}

	h, ok := harness.ByID(f.harnessID)
	if !ok {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("unknown harness %q (valid: %s)",
			f.harnessID, strings.Join(harness.IDs(), ", "))
	}

	p, ok := profile.ByID(f.profileID)
	if !ok {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("unknown profile %q (valid: %s)",
			f.profileID, strings.Join(profile.IDs(), ", "))
	}

	// Feature resolution: union the profile's features with the harness's
	// required features, then layer --with / --no on top.
	initial := append([]string{}, p.Features...)
	initial = append(initial, h.RequiredFeatures()...)

	resolved, err := feature.Resolve(initial, f.with, f.no)
	if err != nil {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("resolve features: %w", err)
	}

	// Materialize Feature structs in the resolved order.
	feats := make([]feature.Feature, 0, len(resolved.Enabled))
	for _, id := range resolved.Enabled {
		fe, _ := feature.ByID(id)
		feats = append(feats, fe)
	}

	// Extensions selections: validate every requested ID exists for this harness.
	var catEntries []*extensions.Entry
	if len(f.extensionIDs) > 0 {
		all, err := extensions.LoadAll(vibrator.ExtensionsFS)
		if err != nil {
			return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("load extensions: %w", err)
		}
		for _, id := range f.extensionIDs {
			key := h.ID() + "/" + id
			entry, ok := all[key]
			if !ok {
				return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf(
					"extensions entry %q not found for harness %q", id, h.ID())
			}
			catEntries = append(catEntries, entry)
		}
	}

	dfSpec := dockerfile.Spec{
		Harness:         h,
		Profile:         f.profileID,
		Shell:           f.shell,
		Features:        feats,
		Extensions:  catEntries,
		Username:        f.username,
		HostUID:         f.hostUID,
		HostGID:         f.hostGID,
		VibratorVersion: Version,
	}

	wsSpec := workspace.Spec{
		Harness:  h.ID(),
		Profile:  f.profileID,
		Shell:    f.shell,
		Features: resolved.Enabled,
		Extensions:  f.extensionIDs,
	}

	return dfSpec, wsSpec, nil
}

func runBuildDockerfile(cmd *cobra.Command, _ []string) error {
	df, _, err := resolveSpec(&buildDockerfileFlags)
	if err != nil {
		return err
	}
	out, err := dockerfile.Generate(df)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	dest := buildDockerfileFlags.out
	if dest == "" || dest == "-" {
		_, _ = cmd.OutOrStdout().Write(out)
		return nil
	}
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote Dockerfile to %s (%d bytes)\n", dest, len(out))
	return nil
}

func runBuild(cmd *cobra.Command, _ []string) error {
	df, ws, err := resolveSpec(&buildFlagsState)
	if err != nil {
		return err
	}

	out, err := dockerfile.Generate(df)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	tag := buildFlagsState.tag
	if tag == "" {
		tag = workspace.ImageName(ws, workspace.Fingerprint(ws))
	}

	client, err := docker.NewCLIClient()
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}

	// Per-build tempdir is the docker build context. Vibrator-owned
	// templates (shells, scripts) live under templates/ in the repo
	// and get extracted into the tempdir at build time by
	// dockerfile.PrepareBuildContext. The Dockerfile COPYs from there.
	//
	// Previously we used cwd, which streamed the user's whole workspace
	// to the daemon on every build — wasteful and a footgun (a stray
	// Dockerfile in the workspace would override ours via `-f -`'s
	// search semantics).
	ctxDir, cleanup, err := dockerfile.PrepareBuildContext()
	if err != nil {
		return fmt.Errorf("prepare build context: %w", err)
	}
	defer cleanup()

	fmt.Fprintf(cmd.ErrOrStderr(), "Building %s (no-cache=%v)\n", tag, buildFlagsState.noCache)

	return client.Build(context.Background(), docker.BuildSpec{
		DockerfileBytes: out,
		ContextDir:      ctxDir,
		Tag:             tag,
		NoCache:         buildFlagsState.noCache,
		BuildArgs: map[string]string{
			"USERNAME": df.Username,
			"HOST_UID": fmt.Sprintf("%d", df.HostUID),
			"HOST_GID": fmt.Sprintf("%d", df.HostGID),
		},
		Stdout: cmd.OutOrStdout(),
		Stderr: cmd.ErrOrStderr(),
	})
}
