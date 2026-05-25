package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/workspace"
)

// UpdateOptions bundles the CLI-flag state for `vibrate update`. Kept
// minimal — update doesn't need a wizard, doesn't take harness/profile
// flags (everything comes from the workspace .vb), doesn't allow
// extension overrides. The only knobs are stream redirection and the
// vibrator version stamp (for build context labelling).
type UpdateOptions struct {
	// VibratorVersion is the version string stamped into any generated
	// Dockerfile header. Mirrors what `vibrate run` does.
	VibratorVersion string

	// Output streams. nil = use OS defaults.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

// Update is the entry point for `vibrate update`. It updates the
// harness's own CLI in place — either inside a running/existing
// container, or by adding a fresh layer to the image and re-tagging.
//
// Decision tree:
//
//   - Container running                → docker exec <update-cmd> and exit.
//   - Container exited / created /     → docker start, then exec <update-cmd>.
//     dead / stopped
//   - Container missing, image exists  → build a new "FROM <image> RUN <update-cmd>"
//                                         image, re-tag with the same name.
//                                         Old image becomes dangling.
//   - Neither container nor image      → error — nothing to update.
//
// Important: when a container is updated, the change lives ONLY in
// that container — the underlying image is unchanged. To persist
// across container recreations, either:
//   - Remove the container, then re-run `vibrate update` (image-layer path).
//   - Use `vibrate --rebuild` for a full clean rebuild from the
//     Dockerfile.
//
// This split keeps `vibrate update` predictable: it does one fast
// thing per invocation and doesn't surprise the user with a docker
// commit or a long rebuild.
func Update(ctx context.Context, opts UpdateOptions) error {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}

	// Reuse the run-time loader to find the workspace + pin. A missing
	// pin is a hard error here — update has nothing to act on without
	// knowing which harness lives in this workspace.
	wsDir, pin, _, err := loadWorkspaceAndPin(Options{
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Stdin:  opts.Stdin,
	})
	if err != nil {
		return err
	}
	if pin.Harness == "" {
		return errors.New("no .vb pin in this workspace — run `vibrate` first to bootstrap")
	}

	h, ok := harness.ByID(pin.Harness)
	if !ok {
		return fmt.Errorf("harness %q in pin is not registered (build error?)", pin.Harness)
	}
	updateCmd := h.UpdateCommand()
	if len(updateCmd) == 0 {
		return fmt.Errorf("harness %q doesn't support in-place updates — use `vibrate --rebuild` instead",
			pin.Harness)
	}

	// Compute the image tag + container name the same way Run() does —
	// they're derived from the workspace spec's fingerprint.
	_, wsSpec, err := buildSpecs(pin, Options{
		Username:        defaultUsername(Options{}),
		HostUID:         defaultUID(Options{}),
		HostGID:         defaultGID(Options{}),
		VibratorVersion: opts.VibratorVersion,
	})
	if err != nil {
		return fmt.Errorf("build specs: %w", err)
	}
	fp := workspace.Fingerprint(wsSpec)
	imageTag := workspace.ImageName(wsSpec, fp)
	containerName := workspace.ContainerName(wsDir, fp)

	dockerCli, err := docker.NewCLIClient()
	if err != nil {
		return err
	}

	status, err := dockerCli.ContainerStatus(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	switch status {
	case "running":
		fmt.Fprintf(opts.Stderr, "→ Container %s running — updating in place (%s)\n",
			containerName, joinForDisplay(updateCmd))
		return execUpdateInContainer(ctx, dockerCli, containerName, wsDir, updateCmd, opts)

	case "exited", "created", "dead":
		fmt.Fprintf(opts.Stderr, "→ Container %s (%s) — starting, then updating\n", containerName, status)
		if err := dockerCli.Start(ctx, containerName); err != nil {
			return fmt.Errorf("docker start: %w", err)
		}
		return execUpdateInContainer(ctx, dockerCli, containerName, wsDir, updateCmd, opts)

	case "":
		// No container. Update via image layer if the image exists,
		// otherwise nothing to do.
		exists, err := dockerCli.ImageExists(ctx, imageTag)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("no container and no image for this workspace (%s) — run `vibrate` first",
				imageTag)
		}
		return updateImageInPlace(ctx, dockerCli, imageTag, updateCmd, opts)

	default:
		return fmt.Errorf("unexpected container status %q for %s", status, containerName)
	}
}

// execUpdateInContainer runs the harness's update command inside a
// running container via `docker exec -it`. Stdio is forwarded so the
// user sees the update's progress live; the call returns when the
// command exits.
//
// We DO NOT wrap with /usr/local/bin/claude-exec here — that wrapper
// is for interactive session starts (it refreshes the MCP integration
// manifest). The update command is a one-shot, no MCP needed.
func execUpdateInContainer(ctx context.Context, dc docker.Client,
	containerName, wsDir string, updateCmd []string, opts UpdateOptions,
) error {
	err := dc.Exec(ctx, docker.ExecSpec{
		Container:   containerName,
		Interactive: true,
		WorkingDir:  wsDir,
		Cmd:         updateCmd,
		Stdin:       opts.Stdin,
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
	})
	if err != nil {
		return fmt.Errorf("update inside container: %w", err)
	}
	fmt.Fprintf(opts.Stderr,
		"\n→ Update complete — change is in the container only. "+
			"To persist into the image, remove the container and rerun `vibrate update`.\n")
	return nil
}

// updateImageInPlace builds a tiny one-layer image (FROM <existing> +
// RUN <update-cmd>) and re-tags the result with the same image name.
// The old image is left untagged (dangling); Docker's layer cache
// keeps all but the topmost layer, so the rebuild is fast.
//
// Failure modes:
//
//   - The update command itself errors inside the container → docker
//     build returns a non-zero exit; the old image keeps its tag.
//   - The network is down → docker build's RUN fails fast; no retag.
//
// Either way, the old image stays valid — `-t same-tag` only re-points
// the tag on a successful build.
func updateImageInPlace(ctx context.Context, dc docker.Client,
	imageTag string, updateCmd []string, opts UpdateOptions,
) error {
	dfBytes := dockerfileForUpdate(imageTag, updateCmd, opts.VibratorVersion)

	// Empty build context — FROM doesn't need files, but docker build
	// still expects a directory argument. A tempdir is safer than
	// re-using the user's workspace (which would stream possibly-large
	// trees to the daemon for no reason).
	tmpDir, err := os.MkdirTemp("", "vibrate-update-")
	if err != nil {
		return fmt.Errorf("create build context tempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintf(opts.Stderr, "→ No container — updating image %s by adding a layer (%s)\n",
		imageTag, joinForDisplay(updateCmd))

	if err := dc.Build(ctx, docker.BuildSpec{
		DockerfileBytes: dfBytes,
		ContextDir:      tmpDir,
		Tag:             imageTag,
		// NoCache=false (default) — we WANT the cache so re-running
		// the same update is fast. The RUN step is invalidated when
		// its base image changes, so this still does fresh work when
		// the underlying image moves.
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	}); err != nil {
		return fmt.Errorf("docker build update layer: %w", err)
	}

	fmt.Fprintf(opts.Stderr, "→ Image %s updated. Next `vibrate` will use it.\n", imageTag)
	return nil
}

// dockerfileForUpdate generates the two-line Dockerfile that turns
// <imageTag> into <imageTag>-with-update via one extra RUN layer.
// Uses the JSON exec form for RUN so we don't have to worry about
// shell quoting of the update argv.
//
// Header comments make `docker history` debuggable when someone is
// wondering "why does this image have an unexplained extra layer".
func dockerfileForUpdate(imageTag string, updateCmd []string, vibratorVersion string) []byte {
	argv, _ := json.Marshal(updateCmd) // []byte safe for printf
	version := vibratorVersion
	if version == "" {
		version = "dev"
	}
	return []byte(fmt.Sprintf(`# syntax=docker/dockerfile:1.7
# Generated by `+"`vibrate update`"+` %s at %s.
# Previous image tag: %s
FROM %s
RUN %s
`,
		version,
		time.Now().UTC().Format(time.RFC3339),
		imageTag,
		imageTag,
		argv,
	))
}

// joinForDisplay renders a command argv as a single shell-ish string
// for user-facing messages. Doesn't try to be a full quoter — purely
// cosmetic.
func joinForDisplay(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	out := argv[0]
	for _, a := range argv[1:] {
		out += " " + a
	}
	return out
}
