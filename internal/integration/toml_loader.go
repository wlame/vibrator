package integration

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/wlame/vibrator/internal/harness"
)

// tomlIntegration is the on-disk schema for a user-defined integration
// (~/.config/vibrator/integrations/*.toml). It mirrors the Integration
// struct narrowly — only the fields TOML authors are expected to
// supply by hand (no Setup hook, no WorkspaceDriver — those need Go).
type tomlIntegration struct {
	Integration tomlIdentity `toml:"integration"`

	Probe   *tomlProbe   `toml:"probe"`
	Runtime tomlRuntimes `toml:"runtime"`
	Wiring  []tomlWiring `toml:"wiring"`
}

type tomlIdentity struct {
	ID       string `toml:"id"`
	Name     string `toml:"name"`
	Summary  string `toml:"summary"`
	Docs     string `toml:"docs"`
	Category string `toml:"category"`
}

type tomlProbe struct {
	Type           string `toml:"type"`            // "http" (only one supported today)
	URL            string `toml:"url"`             // for http
	TimeoutSeconds int    `toml:"timeout_seconds"` // 0 = default 2 s
}

type tomlRuntimes struct {
	Docker   *tomlDockerRuntime   `toml:"docker"`
	External *tomlExternalRuntime `toml:"external"`
}

type tomlDockerRuntime struct {
	Image         string            `toml:"image"`
	ContainerName string            `toml:"container_name"`
	Ports         []string          `toml:"ports"`
	Volumes       []string          `toml:"volumes"`
	Env           map[string]string `toml:"env"`
	AddHosts      []string          `toml:"add_hosts"`
	Restart       string            `toml:"restart"`
	Network       string            `toml:"network"`
	Command       []string          `toml:"command"`
	Args          []string          `toml:"args"`
}

type tomlExternalRuntime struct {
	Instructions string `toml:"instructions"`
}

type tomlWiring struct {
	Harness string            `toml:"harness"`
	MCP     *tomlMCPWiring    `toml:"mcp"`
	Env     map[string]string `toml:"env"`
}

type tomlMCPWiring struct {
	Name  string        `toml:"name"`
	HTTP  *tomlMCPHTTP  `toml:"http"`
	Stdio *tomlMCPStdio `toml:"stdio"`
}

type tomlMCPHTTP struct {
	URL     string            `toml:"url"`
	Headers map[string]string `toml:"headers"`
}

type tomlMCPStdio struct {
	Command []string          `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
}

// ── Errors ──────────────────────────────────────────────────────────────

// LoadError captures one failed TOML file load. The loader continues
// past individual file errors so a single bad descriptor doesn't
// silently break the whole picker.
type LoadError struct {
	Path string
	Err  error
}

func (le LoadError) Error() string { return fmt.Sprintf("%s: %v", le.Path, le.Err) }

// loadErrorsMu guards loadErrors. Accessed from init-time loaders and
// from `vibrate integrations list` (which surfaces them).
var (
	loadErrorsMu sync.Mutex
	loadErrors   []LoadError
)

// LoadErrors returns a snapshot of TOML-loader errors collected since
// program start. Used by the `vibrate integrations list` command to
// surface bad descriptors without making them fatal.
func LoadErrors() []LoadError {
	loadErrorsMu.Lock()
	defer loadErrorsMu.Unlock()
	out := make([]LoadError, len(loadErrors))
	copy(out, loadErrors)
	return out
}

func recordLoadError(le LoadError) {
	loadErrorsMu.Lock()
	defer loadErrorsMu.Unlock()
	loadErrors = append(loadErrors, le)
}

// ── Loader ──────────────────────────────────────────────────────────────

// LoadFromDir scans dir for *.toml files and registers an Integration
// for each one that parses successfully. Files with parse errors are
// collected via LoadErrors() rather than aborting the load — the
// picker can still show every valid integration.
//
// Returns the number of integrations successfully registered.
//
// Safe to call when dir doesn't exist: returns (0, nil) silently.
func LoadFromDir(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}

	// Stable order makes `list` output deterministic.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := loadOne(path); err != nil {
			recordLoadError(LoadError{Path: path, Err: err})
			continue
		}
		loaded++
	}
	return loaded, nil
}

// loadOne parses one TOML file and registers the resulting
// Integration. Returns an error without registering on parse or
// validation failures.
//
// Uses a named return so the recovery defer can convert a Register()
// panic into a returned error — the caller treats that as a "failed
// to load" and records via LoadErrors, keeping the loaded-count
// accurate (no double-counting).
func loadOne(path string) (err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var ti tomlIntegration
	if _, err := toml.Decode(string(data), &ti); err != nil {
		return fmt.Errorf("toml parse: %w", err)
	}
	integ, err := tomlToIntegration(&ti)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("register: %v", r)
		}
	}()
	Register(integ)
	return nil
}

// tomlToIntegration converts the on-disk schema to an in-memory
// Integration. Validates required fields up front so authors get
// clear errors instead of mysterious nil-deref crashes later.
func tomlToIntegration(ti *tomlIntegration) (*Integration, error) {
	if strings.TrimSpace(ti.Integration.ID) == "" {
		return nil, fmt.Errorf("[integration].id is required")
	}
	if strings.TrimSpace(ti.Integration.Name) == "" {
		return nil, fmt.Errorf("[integration].name is required")
	}

	out := &Integration{
		ID:       ti.Integration.ID,
		Name:     ti.Integration.Name,
		Summary:  ti.Integration.Summary,
		DocsURL:  ti.Integration.Docs,
		Category: ti.Integration.Category,
	}

	// Probe — only http supported in v0. Empty probe is allowed:
	// integrations that don't expose an HTTP endpoint don't probe.
	if p := ti.Probe; p != nil && p.URL != "" {
		probeType := p.Type
		if probeType == "" {
			probeType = "http"
		}
		if probeType != "http" {
			return nil, fmt.Errorf("[probe].type=%q not supported (only http)", probeType)
		}
		timeout := time.Duration(p.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 2 * time.Second
		}
		url := p.URL
		out.ProbeFn = func(_ context.Context) (Probe, error) {
			return HTTPProbe{URL: url, Timeout: timeout}, nil
		}
	}

	// Runtimes — translate each declared block into a HostRuntime.
	if r := ti.Runtime.Docker; r != nil {
		if r.Image == "" {
			return nil, fmt.Errorf("[runtime.docker].image is required")
		}
		if r.ContainerName == "" {
			return nil, fmt.Errorf("[runtime.docker].container_name is required")
		}
		out.Runtimes = append(out.Runtimes, &DockerRuntime{
			Image:         r.Image,
			ContainerName: r.ContainerName,
			Ports:         r.Ports,
			Volumes:       r.Volumes,
			Env:           r.Env,
			AddHosts:      r.AddHosts,
			Restart:       r.Restart,
			Network:       r.Network,
			Command:       r.Command,
			Args:          r.Args,
		})
	}
	if r := ti.Runtime.External; r != nil {
		out.Runtimes = append(out.Runtimes, &ExternalRuntime{
			Instructions: r.Instructions,
		})
	}

	// Wiring — convert each entry.
	for _, w := range ti.Wiring {
		if w.Harness == "" {
			return nil, fmt.Errorf("[[wiring]].harness is required")
		}
		if w.Harness != "*" {
			if _, ok := harness.ByID(w.Harness); !ok {
				return nil, fmt.Errorf("[[wiring]].harness %q is not a known harness (valid: %s, or \"*\")",
					w.Harness, strings.Join(harness.IDs(), ", "))
			}
		}
		entry := Wiring{Harness: w.Harness, EnvVars: w.Env}
		if w.MCP != nil {
			if w.MCP.Name == "" {
				return nil, fmt.Errorf("[wiring.mcp].name is required when [wiring.mcp] is declared")
			}
			m := &MCPWiring{Name: w.MCP.Name}
			if h := w.MCP.HTTP; h != nil && h.URL != "" {
				m.HTTP = &MCPHTTP{URL: h.URL, Headers: h.Headers}
			}
			if s := w.MCP.Stdio; s != nil && len(s.Command) > 0 {
				m.Stdio = &MCPStdio{
					Command: s.Command,
					Args:    s.Args,
					Env:     s.Env,
				}
			}
			if m.HTTP == nil && m.Stdio == nil {
				return nil, fmt.Errorf("[wiring.mcp] needs at least one of http or stdio")
			}
			entry.MCP = m
		}
		out.Wiring = append(out.Wiring, entry)
	}

	return out, nil
}
