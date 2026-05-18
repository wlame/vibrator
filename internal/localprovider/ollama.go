package localprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"time"
)

// ollamaProvider talks to the Ollama HTTP API.
//
// Ollama API endpoints used:
//   GET  /api/tags                — list locally-downloaded models
//   POST /api/pull (body: {"name": "<model>"}) — pull a model
//
// Background-startup pattern: `ollama serve` runs in the foreground;
// we exec it with os/exec.Cmd.Start() so the process detaches from
// the vibrate parent process. Stdout/stderr are redirected to
// /dev/null so the wizard doesn't get spammed.
type ollamaProvider struct{}

// Static interface check — if Provider grows a new method, the build
// breaks here before tests do.
var _ Provider = (*ollamaProvider)(nil)

func (ollamaProvider) ID() string         { return "ollama" }
func (ollamaProvider) Name() string       { return "Ollama" }
func (ollamaProvider) DefaultURL() string { return "http://host.docker.internal:11434" }
func (ollamaProvider) HostBinary() string { return "ollama" }

func (p ollamaProvider) Probe(ctx context.Context, url string) error {
	if url == "" {
		url = p.DefaultURL()
	}
	return ollamaGetJSON(ctx, url+"/api/tags", nil, 2*time.Second)
}

// ollamaTagsResponse is the shape of /api/tags. Many other fields exist
// (size, modified_at, etc.) but we only need the names.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (p ollamaProvider) ListLocalModels(ctx context.Context, url string) ([]string, error) {
	if url == "" {
		url = p.DefaultURL()
	}
	var resp ollamaTagsResponse
	if err := ollamaGetJSON(ctx, url+"/api/tags", &resp, 4*time.Second); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names, nil
}

func (p ollamaProvider) EnsureRunning(ctx context.Context, url, model string) error {
	if url == "" {
		url = p.DefaultURL()
	}

	// Fast path: if it's already up, we're done.
	if err := p.Probe(ctx, url); err == nil {
		// Still want to make sure the requested model is available.
		if model != "" {
			return ollamaEnsureModel(ctx, url, model)
		}
		return nil
	}

	// Slow path: start the daemon. Requires `ollama` on PATH.
	if _, err := exec.LookPath(p.HostBinary()); err != nil {
		return fmt.Errorf("ollama: server not reachable at %s and `ollama` binary not on PATH (%w)", url, err)
	}
	if err := commandStarter(p.HostBinary(), "serve"); err != nil {
		return fmt.Errorf("ollama: failed to spawn `ollama serve`: %w", err)
	}

	// Poll until healthy, with a generous cap. Ollama's cold-start can
	// take a few seconds on slower disks.
	if err := waitForServer(ctx, func(ctx context.Context) error { return p.Probe(ctx, url) },
		10*time.Second, 250*time.Millisecond); err != nil {
		return fmt.Errorf("ollama: spawned but never became reachable at %s: %w", url, err)
	}

	if model != "" {
		return ollamaEnsureModel(ctx, url, model)
	}
	return nil
}

// ollamaEnsureModel checks if `model` is already in the tags list; if
// not, POSTs /api/pull to download it. Pulling can be slow (multi-GB
// downloads) — we stream the response until the server reports "success".
func ollamaEnsureModel(ctx context.Context, url, model string) error {
	tags, err := (ollamaProvider{}).ListLocalModels(ctx, url)
	if err != nil {
		return fmt.Errorf("ollama: list models: %w", err)
	}
	for _, t := range tags {
		if t == model {
			return nil // already have it
		}
	}

	// Pull is a long-running streaming request. Use a longer-than-default
	// timeout via the ctx we were handed.
	pullReq := struct {
		Name string `json:"name"`
	}{Name: model}
	body, _ := json.Marshal(pullReq)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/pull",
		newBytesReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: pull %s: %w", model, err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama: pull %s returned HTTP %d", model, resp.StatusCode)
	}
	return nil
}

func init() {
	Register(ollamaProvider{})
}
