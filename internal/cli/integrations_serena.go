package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	serena "github.com/wlame/vibrator/internal/integration/serena"
)

// runIntegrationsSerena is the entry point for `vibrate integrations serena`.
// It reads the current daemon state and branches into manage or start flows.
func runIntegrationsSerena(cmd *cobra.Command, _ []string) error {
	c := newColors(!isTerminal(cmd.OutOrStdout()))
	out := cmd.OutOrStdout()
	port := resolveSerenaPort()

	fmt.Fprintf(out, "\n%s%s%s\n", c.bold, "Serena MCP — host server", c.reset)
	fmt.Fprintln(out, strings.Repeat("─", 42))
	fmt.Fprintf(out, "  Port:  %d  (override via $SERENA_PORT)\n\n", port)

	state, err := serena.Read(port)
	if err != nil {
		return fmt.Errorf("read daemon state: %w", err)
	}

	// Stale PID file — clean up message before falling through to start flow.
	if state.Status == serena.StatusStale {
		fmt.Fprintf(out, "  %s⚠%s  Stale PID file found (PID %d no longer running) — cleaned up.\n\n",
			c.yellow, c.reset, state.PID)
		state.Status = serena.StatusStopped
	}

	// If a Docker container is running, treat it as "managed by docker".
	if serena.ContainerRunning() {
		return serenaManageDocker(cmd, port, c)
	}

	if state.Status == serena.StatusRunning {
		return serenaManageProcess(cmd, state, c)
	}

	// Probe anyway — maybe a manually-started server is reachable.
	if serena.Probe(port) {
		fmt.Fprintf(out, "  %s✓%s Serena is reachable (externally managed).\n", c.green, c.reset)
		fmt.Fprintf(out, "    Container URL: http://host.docker.internal:%d/mcp\n\n", port)
		return nil
	}

	fmt.Fprintf(out, "  %s✗%s Serena is not running.\n\n", c.red, c.reset)
	return serenaStartFlow(cmd, port, c)
}

// serenaManageProcess presents management options when Serena is running as a process.
func serenaManageProcess(cmd *cobra.Command, state serena.ProcessState, c colors) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "  %s✓%s Running as background process (PID %d)\n", c.green, c.reset, state.PID)
	fmt.Fprintf(out, "    Log: %s\n\n", state.LogFile)

	var action string
	for {
		form := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Manage Serena").
				Options(
					huh.NewOption("Show container URL", "url"),
					huh.NewOption("Tail log (last 40 lines)", "tail"),
					huh.NewOption("Stop", "stop"),
					huh.NewOption("Restart", "restart"),
					huh.NewOption("Done", "done"),
				).
				Value(&action),
		)).WithTheme(huh.ThemeCharm())

		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch action {
		case "url":
			fmt.Fprintf(out, "\n  Container URL: http://host.docker.internal:%d/mcp\n\n", state.Port)

		case "tail":
			log, err := serena.TailLog(8192)
			if err != nil {
				fmt.Fprintf(out, "  %slog error: %v%s\n\n", c.red, err, c.reset)
			} else if log == "" {
				fmt.Fprintf(out, "  %s(log is empty)%s\n\n", c.dim, c.reset)
			} else {
				fmt.Fprintf(out, "\n%s\n\n", log)
			}

		case "stop":
			fmt.Fprintf(out, "  Stopping PID %d …\n", state.PID)
			if err := serena.Stop(state); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			fmt.Fprintf(out, "  %s✓%s Serena stopped.\n\n", c.green, c.reset)
			return nil

		case "restart":
			fmt.Fprintf(out, "  Stopping PID %d …\n", state.PID)
			if err := serena.Stop(state); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			fmt.Fprintf(out, "  Starting …\n")
			pid, err := serena.Start(state.Port)
			if err != nil {
				return fmt.Errorf("restart: %w", err)
			}
			fmt.Fprintf(out, "  %s✓%s Restarted (PID %d) — waiting for server …\n", c.green, c.reset, pid)
			serenaAwaitReady(out, state.Port, c)
			// Refresh state for next loop iteration.
			state.PID = pid

		case "done":
			return nil
		}
	}
}

// serenaManageDocker presents management options when Serena is running in Docker.
func serenaManageDocker(cmd *cobra.Command, port int, c colors) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "  %s✓%s Running as Docker container (%s)\n\n", c.green, c.reset, serena.ContainerName)

	var action string
	for {
		form := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Manage Serena container").
				Options(
					huh.NewOption("Show container URL", "url"),
					huh.NewOption("Tail container logs", "tail"),
					huh.NewOption("Stop and remove container", "stop"),
					huh.NewOption("Restart container", "restart"),
					huh.NewOption("Done", "done"),
				).
				Value(&action),
		)).WithTheme(huh.ThemeCharm())

		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch action {
		case "url":
			fmt.Fprintf(out, "\n  Container URL: http://host.docker.internal:%d/mcp\n\n", port)

		case "tail":
			fmt.Fprintf(out, "\n")
			tailCmd := exec.Command("docker", "logs", "--tail=40", serena.ContainerName)
			tailCmd.Stdout = out
			tailCmd.Stderr = out
			_ = tailCmd.Run()
			fmt.Fprintln(out)

		case "stop":
			fmt.Fprintf(out, "  Stopping container %s …\n", serena.ContainerName)
			if err := serena.StopDocker(); err != nil {
				return err
			}
			fmt.Fprintf(out, "  %s✓%s Container stopped and removed.\n\n", c.green, c.reset)
			return nil

		case "restart":
			fmt.Fprintf(out, "  Restarting container %s …\n", serena.ContainerName)
			if err := exec.Command("docker", "restart", serena.ContainerName).Run(); err != nil {
				return fmt.Errorf("docker restart: %w", err)
			}
			fmt.Fprintf(out, "  %s✓%s Container restarted — waiting for server …\n", c.green, c.reset)
			serenaAwaitReady(out, port, c)

		case "done":
			return nil
		}
	}
}

// serenaStartFlow presents the "how would you like to start Serena?" picker.
func serenaStartFlow(cmd *cobra.Command, port int, c colors) error {
	var choice string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("How would you like to start Serena?").
			Options(
				huh.NewOption("Background process  (uvx, survives terminal close)", "process"),
				huh.NewOption("Docker container  (auto-restarts on system boot)", "docker"),
				huh.NewOption("Skip — I'll start it manually", "skip"),
			).
			Value(&choice),
	)).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		return err
	}

	switch choice {
	case "process":
		return serenaStartProcess(cmd, port, c)
	case "docker":
		return serenaStartDocker(cmd, port, c)
	case "skip":
		serenaShowInstructions(cmd.OutOrStdout(), port, c)
	}
	return nil
}

func serenaStartProcess(cmd *cobra.Command, port int, c colors) error {
	out := cmd.OutOrStdout()

	if _, err := exec.LookPath("uvx"); err != nil {
		fmt.Fprintf(out, "\n  %s✗%s uvx not found on PATH.\n", c.red, c.reset)
		fmt.Fprintf(out, "  Install uv first:  curl -LsSf https://astral.sh/uv/install.sh | sh\n\n")
		return nil
	}

	logPath, _ := serena.LogPath()
	fmt.Fprintf(out, "\n  Spawning Serena on port %d …\n", port)
	fmt.Fprintf(out, "  Logs → %s\n", logPath)
	fmt.Fprintf(out, "  %s(first run downloads dependencies — may take up to 60 s)%s\n\n", c.dim, c.reset)

	pid, err := serena.Start(port)
	if err != nil {
		return fmt.Errorf("start Serena: %w", err)
	}
	fmt.Fprintf(out, "  %s✓%s Spawned (PID %d)\n", c.green, c.reset, pid)

	serenaAwaitReady(out, port, c)
	return nil
}

func serenaStartDocker(cmd *cobra.Command, port int, c colors) error {
	out := cmd.OutOrStdout()

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not on PATH")
	}

	if serena.ContainerExists() {
		fmt.Fprintf(out, "\n  %sContainer %s already exists.%s\n", c.yellow, serena.ContainerName, c.reset)
		fmt.Fprintf(out, "  Run %sdocker start %s%s to restart it,\n", c.bold, serena.ContainerName, c.reset)
		fmt.Fprintf(out, "  or use %svibrate integrations serena%s to manage it.\n\n", c.bold, c.reset)
		return nil
	}

	fmt.Fprintf(out, "\n  Starting Docker container %s …\n", serena.ContainerName)
	fmt.Fprintf(out, "  Image: ghcr.io/astral-sh/uv:python3.12-bookworm-slim\n")
	fmt.Fprintf(out, "  %s(first run pulls the image and downloads Serena — may take 1–2 min)%s\n\n",
		c.dim, c.reset)

	if err := serena.StartDocker(port); err != nil {
		return fmt.Errorf("start Docker container: %w", err)
	}
	fmt.Fprintf(out, "  %s✓%s Container started (auto-restart on boot).\n", c.green, c.reset)

	serenaAwaitReady(out, port, c)
	return nil
}

// serenaAwaitReady polls Probe for up to 90 s and prints the result.
func serenaAwaitReady(out interface{ Write([]byte) (int, error) }, port int, c colors) {
	fmt.Fprintf(out, "  Waiting for Serena to become reachable …")
	ok := serena.PollReady(port, 90*time.Second)
	if ok {
		fmt.Fprintf(out, "\r  %s✓%s Serena is up at http://127.0.0.1:%d/mcp                         \n",
			c.green, c.reset, port)
		fmt.Fprintf(out, "    Containers connect via: http://host.docker.internal:%d/mcp\n", port)
		fmt.Fprintf(out, "    claude-exec.sh will switch to HTTP transport on next shell entry.\n\n")
	} else {
		fmt.Fprintf(out, "\r  %s✗%s Serena did not become reachable within 90 s.                    \n",
			c.red, c.reset)
		log, _ := serena.TailLog(2048)
		if log != "" {
			fmt.Fprintf(out, "\n  Last log lines:\n%s\n\n", log)
		}
		fmt.Fprintf(out, "  Check the log for errors and try again.\n\n")
	}
}

// serenaShowInstructions prints manual startup instructions.
func serenaShowInstructions(out interface{ Write([]byte) (int, error) }, port int, c colors) {
	fmt.Fprintf(out, `
  %sManual startup instructions:%s

    # Foreground (for log inspection):
    uvx --from 'git+https://github.com/oraios/serena' \
      serena start-mcp-server \
      --transport=streamable-http --host=0.0.0.0 --port=%d \
      --enable-web-dashboard=true

    # Docker (auto-restart):
    docker run -d --name %s --restart unless-stopped \
      -p 127.0.0.1:%d:%d \
      -v vibrate-serena-cache:/root/.cache/uv \
      -v ~/.serena:/root/.serena \
      ghcr.io/astral-sh/uv:python3.12-bookworm-slim \
      uvx --from 'git+https://github.com/oraios/serena' \
        serena start-mcp-server \
        --transport=streamable-http --host=0.0.0.0 --port=%d \
        --enable-web-dashboard=true

  The container's claude-exec.sh probes http://host.docker.internal:%d/mcp
  on every session start and auto-switches the Serena MCP transport.
  No container rebuild needed after starting the host server.

`, c.bold, c.reset, port, serena.ContainerName, port, port, port, port)
}

// resolveSerenaPort returns $SERENA_PORT if set, else DefaultPort.
func resolveSerenaPort() int {
	if s := os.Getenv("SERENA_PORT"); s != "" {
		if p, err := strconv.Atoi(s); err == nil && p > 0 {
			return p
		}
	}
	return serena.DefaultPort
}
