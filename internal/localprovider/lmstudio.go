package localprovider

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"time"
)

// lmstudioProvider talks to LM Studio's OpenAI-compatible HTTP API.
//
// LM Studio's "Local Server" exposes /v1/models (and /v1/chat/completions
// etc.) on a configurable port (default 1234). It also ships a CLI named
// `lms` that can start the server from the command line:
//
//	lms server start
//
// Unlike Ollama, the LMS CLI returns immediately (the server is
// background-spawned by `lms` itself), so we don't need to detach via
// commandStarter — a normal exec.Run() is enough.
type lmstudioProvider struct{}

var _ Provider = (*lmstudioProvider)(nil)

func (lmstudioProvider) ID() string         { return "lmstudio" }
func (lmstudioProvider) Name() string       { return "LM Studio" }
func (lmstudioProvider) DefaultURL() string { return "http://host.docker.internal:1234" }
func (lmstudioProvider) HostBinary() string { return "lms" }

func (p lmstudioProvider) Probe(ctx context.Context, url string) error {
	if url == "" {
		url = p.DefaultURL()
	}
	return ollamaGetJSON(ctx, url+"/v1/models", nil, 2*time.Second)
}

// lmstudioModelsResponse mirrors OpenAI's /v1/models shape. We only need
// the IDs.
type lmstudioModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (p lmstudioProvider) ListLocalModels(ctx context.Context, url string) ([]string, error) {
	if url == "" {
		url = p.DefaultURL()
	}
	var resp lmstudioModelsResponse
	if err := ollamaGetJSON(ctx, url+"/v1/models", &resp, 4*time.Second); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		names = append(names, m.ID)
	}
	sort.Strings(names)
	return names, nil
}

func (p lmstudioProvider) EnsureRunning(ctx context.Context, url, model string) error {
	if url == "" {
		url = p.DefaultURL()
	}

	if err := p.Probe(ctx, url); err == nil {
		// LM Studio model is loaded in-process by user; we can't reliably
		// "load model X" via CLI without knowing whether they want to evict
		// the current one. So we just verify the server is up.
		return nil
	}

	if _, err := exec.LookPath(p.HostBinary()); err != nil {
		return fmt.Errorf("lmstudio: server not reachable at %s and `lms` binary not on PATH (%w)", url, err)
	}

	// `lms server start` exits after launching the server. We don't use
	// commandStarter (which detaches) because lms already detaches.
	cmd := exec.CommandContext(ctx, p.HostBinary(), "server", "start")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lmstudio: `lms server start` failed: %w (output: %s)", err, string(out))
	}

	if err := waitForServer(ctx, func(ctx context.Context) error { return p.Probe(ctx, url) },
		8*time.Second, 250*time.Millisecond); err != nil {
		return fmt.Errorf("lmstudio: started but never became reachable at %s: %w", url, err)
	}

	// `model` is informational here — the user must load it via LM Studio
	// GUI or `lms load <model>`. We don't auto-load to avoid evicting
	// the user's already-loaded model.
	_ = model
	return nil
}

func init() {
	Register(lmstudioProvider{})
}
