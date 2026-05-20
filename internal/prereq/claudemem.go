package prereq

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/wlame/vibrator/internal/docker"
)

// ClaudeMemPrereqID is the canonical ID for the claude-mem server-beta
// prerequisite. Referenced from `extensions/claude-code/claude-mem.md`'s
// `prereq:` frontmatter field.
const ClaudeMemPrereqID = "claude-mem-server-beta"

// claudeMemPostgresImage is the docker image used for the one-shot psql
// container. Pinned so behavior doesn't drift across host installs.
const claudeMemPostgresImage = "postgres:16-alpine"

// claudeMemDefaultTeam is the team name minted by Bootstrap when the user
// hasn't configured an override.
const claudeMemDefaultTeam = "vibrators"

// claudeMemKeyPrefix is the prefix on minted plaintext keys. Matches the
// upstream claude-mem server's convention so it can recognize the format
// in audit logs.
const claudeMemKeyPrefix = "cmem_"

// claudeMemKeyRandomBytes is the number of random bytes hex-encoded into
// each key (24 bytes → 48 hex chars).
const claudeMemKeyRandomBytes = 24

// localhostRewrite captures localhost / 127.0.0.1 hosts in a postgres URL
// so the one-shot container can reach the host's daemon via
// host.docker.internal. Single-quoted Go regex form; the corresponding sed
// in the bash impl uses '#' as the delimiter because '|' alternation
// confuses BSD sed. We have no such constraint here.
var localhostRewrite = regexp.MustCompile(`//([^/@]*@)?(localhost|127\.0\.0\.1)([:/])`)

// hostInternalRewrite is the inverse: it captures host.docker.internal so
// host-side probes (which can't resolve that DNS name — Docker Desktop only
// injects it inside containers) can fall back to 127.0.0.1. Anchored the
// same way as localhostRewrite to preserve any userinfo and the trailing
// `:port` or `/path` separator.
var hostInternalRewrite = regexp.MustCompile(`//([^/@]*@)?host\.docker\.internal([:/])`)

// ClaudeMemAdminConfig is the host-only configuration that controls how the
// claude-mem prereq bootstraps and verifies. Lives at
// `~/.config/vibrator/claude-mem.toml` (overridable via
// VIBRATOR_CLAUDE_MEM_CONFIG).
//
// SECURITY: DatabaseURL is the host's postgres connection string with
// privileged credentials. It NEVER crosses into the workspace container.
// Only the project-scoped Bearer token minted by Bootstrap does.
type ClaudeMemAdminConfig struct {
	// Runtime mirrors `CLAUDE_MEM_RUNTIME` (typically "server-beta"). Stored
	// so `vibrate prereqs status` can show what mode the container will be
	// configured for.
	Runtime string `toml:"runtime"`

	// ServerURL is the claude-mem server-beta endpoint reachable from the
	// host AND from inside the container. Use `http://host.docker.internal:<port>`
	// so the same value works on either side.
	ServerURL string `toml:"server_url"`

	// DatabaseURL is the postgres DSN for Bootstrap. Optional — without it,
	// Bootstrap is unavailable and users must mint keys manually.
	DatabaseURL string `toml:"database_url,omitempty"`

	// TeamName, when non-empty, overrides claudeMemDefaultTeam. Useful when
	// a host runs multiple separated team scopes.
	TeamName string `toml:"team_name,omitempty"`
}

// LoadClaudeMemAdminConfig reads the admin config from disk. Returns
// (nil, os.ErrNotExist) if the file is missing — that's a normal "not
// configured yet" state, not an error condition.
//
// The config path is resolved in this order:
//  1. VIBRATOR_CLAUDE_MEM_CONFIG env var (if set)
//  2. $XDG_CONFIG_HOME/vibrator/claude-mem.toml (if XDG_CONFIG_HOME is set)
//  3. $HOME/.config/vibrator/claude-mem.toml
func LoadClaudeMemAdminConfig() (*ClaudeMemAdminConfig, error) {
	path := ClaudeMemAdminConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ClaudeMemAdminConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &cfg, nil
}

// SaveClaudeMemAdminConfig writes cfg to the admin config path, creating
// parent directories if needed, with mode 0600 (the file may contain the
// database_url credential).
func SaveClaudeMemAdminConfig(cfg *ClaudeMemAdminConfig) error {
	path := ClaudeMemAdminConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(cfg); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return os.WriteFile(path, b.Bytes(), 0600)
}

// ClaudeMemAdminConfigPath returns the resolved path used by Load. Split
// out so the CLI can print it in `vibrate prereqs status` even when the
// file is missing.
func ClaudeMemAdminConfigPath() string {
	if override := os.Getenv("VIBRATOR_CLAUDE_MEM_CONFIG"); override != "" {
		return override
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibrator", "claude-mem.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibrator", "claude-mem.toml")
}

// ClaudeMemPrereq returns a fully-wired Prereq for claude-mem.
//
// `cfg` is required for Verifier (we need ServerURL to probe) and for
// Bootstrapper (we need DatabaseURL to mint keys). `client` is required for
// Bootstrapper (the one-shot psql container).
//
// Pass nil for `client` if you only need the verifier (e.g., from the
// wizard's pre-check, where bootstrapping isn't a concern). The returned
// Prereq's Bootstrapper will be nil in that case, signaling "manual setup
// only" to the caller.
func ClaudeMemPrereq(cfg *ClaudeMemAdminConfig, client docker.Client) *Prereq {
	p := &Prereq{
		ID:       ClaudeMemPrereqID,
		Name:     "claude-mem (server-beta runtime)",
		SetupDoc: "extensions/claude-code/claude-mem.md#host-setup",
	}

	if cfg != nil && cfg.ServerURL != "" {
		p.Verifier = HTTPVerify{
			// Stored ServerURL is container-shaped (host.docker.internal:port)
			// because that's what the workspace container ultimately uses.
			// Probes run from the host, where that name doesn't resolve, so
			// rewrite to 127.0.0.1 for the probe only.
			URL:     rewriteForHostProbe(cfg.ServerURL),
			Timeout: 2 * time.Second,
			Hint: "start the claude-mem server-beta stack on the host " +
				"(see extensions/claude-code/claude-mem.md#host-setup)",
		}
	} else {
		// No URL configured → verifier always fails with a setup hint.
		p.Verifier = VerifierFunc(func(context.Context) Result {
			return Result{
				OK:      false,
				Message: "claude-mem admin config missing or has no server_url",
				Hint: fmt.Sprintf("create %s with runtime=\"server-beta\" "+
					"and server_url=\"http://host.docker.internal:<port>\"",
					ClaudeMemAdminConfigPath()),
			}
		})
	}

	if cfg != nil && cfg.DatabaseURL != "" && client != nil {
		p.Bootstrapper = &ClaudeMemBootstrap{
			Docker:        client,
			DatabaseURL:   cfg.DatabaseURL,
			TeamName:      cmDefault(cfg.TeamName, claudeMemDefaultTeam),
			PostgresImage: claudeMemPostgresImage,
		}
	}

	return p
}

// ClaudeMemBootstrap mints a project-scoped API key by running SQL against
// the host's postgres via a one-shot postgres:16-alpine container. The
// host's DSN never leaves the host — it's passed as a `docker run` arg to
// the one-shot, which then connects to the host postgres via
// host.docker.internal. The container in which `claude-mem` ultimately
// runs only ever sees the resulting Bearer token, never the DSN.
type ClaudeMemBootstrap struct {
	// Docker is the docker client used to launch the one-shot psql
	// container. Mockable in tests.
	Docker docker.Client

	// DatabaseURL is the postgres DSN. Will have localhost/127.0.0.1
	// rewritten to host.docker.internal before being passed to the
	// one-shot.
	DatabaseURL string

	// TeamName is the parent team for the minted key. Defaults to
	// "vibrators" if empty.
	TeamName string

	// PostgresImage is the image used for the one-shot. Defaults to
	// postgres:16-alpine.
	PostgresImage string

	// rng is the source for key generation. Defaults to crypto/rand.
	// Test-only knob; not exported.
	rng io.Reader
}

// Bootstrap implements Bootstrapper. Runs the three-step SQL flow (team,
// project, key rotation) against the host postgres via three separate
// one-shot containers — splitting them keeps each error message scoped to a
// single discrete step.
//
// Returned map contains:
//
//	api_key    — the minted plaintext key (cmem_<48-hex>); store in pin
//	team_id    — the team UUID resolved or created
//	project_id — the project UUID resolved or created
//	actor_id   — the actor identifier ("vibrator:<host>:<path>")
func (b *ClaudeMemBootstrap) Bootstrap(ctx context.Context, ws Workspace) (map[string]string, error) {
	if b.Docker == nil {
		return nil, errors.New("claude-mem bootstrap: docker client is nil")
	}
	if b.DatabaseURL == "" {
		return nil, errors.New("claude-mem bootstrap: DatabaseURL is empty")
	}
	if ws.ProjectName == "" {
		return nil, errors.New("claude-mem bootstrap: workspace.ProjectName is empty")
	}

	teamName := b.TeamName
	if teamName == "" {
		teamName = claudeMemDefaultTeam
	}
	image := b.PostgresImage
	if image == "" {
		image = claudeMemPostgresImage
	}

	// 1. Mint key + hash. Key is the value persisted into the pin; hash is
	//    what the server stores. Server compares its stored hash against
	//    SHA256(client-supplied-key) on every request.
	rawKey, err := mintClaudeMemKey(b.rng)
	if err != nil {
		return nil, fmt.Errorf("mint key: %w", err)
	}
	keyHash := sha256Hex(rawKey)

	// 2. Rewrite localhost → host.docker.internal so the one-shot container
	//    can reach the host's postgres.
	rewrittenDSN := rewriteForOneshot(b.DatabaseURL)

	// 3. Upsert team.
	teamID, err := b.runPSQL(ctx, image, rewrittenDSN,
		`SELECT id FROM teams WHERE name = :'name' LIMIT 1`,
		"name", teamName)
	if err != nil {
		return nil, fmt.Errorf("lookup team: %w", err)
	}
	if teamID == "" {
		teamID, err = b.runPSQL(ctx, image, rewrittenDSN,
			`INSERT INTO teams (id, name) VALUES (gen_random_uuid()::text, :'name') RETURNING id`,
			"name", teamName)
		if err != nil {
			return nil, fmt.Errorf("create team: %w", err)
		}
	}
	if teamID == "" {
		return nil, errors.New("team upsert returned empty id")
	}

	// 4. Upsert project.
	projectID, err := b.runPSQL(ctx, image, rewrittenDSN,
		`SELECT id FROM projects WHERE team_id = :'tid' AND name = :'name' LIMIT 1`,
		"tid", teamID, "name", ws.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("lookup project: %w", err)
	}
	if projectID == "" {
		projectID, err = b.runPSQL(ctx, image, rewrittenDSN,
			`INSERT INTO projects (id, team_id, name) VALUES (gen_random_uuid()::text, :'tid', :'name') RETURNING id`,
			"tid", teamID, "name", ws.ProjectName)
		if err != nil {
			return nil, fmt.Errorf("create project: %w", err)
		}
	}
	if projectID == "" {
		return nil, errors.New("project upsert returned empty id")
	}

	// 5. Rotate the key: revoke any prior live key for (team, project,
	//    actor) and insert the new one. Two statements in a single psql
	//    invocation = one transaction.
	actorID := fmt.Sprintf("vibrator:%s:%s", ws.Hostname, ws.Path)
	_, err = b.runPSQL(ctx, image, rewrittenDSN, `
		UPDATE api_keys SET revoked_at = now()
		 WHERE team_id    = :'tid'
		   AND project_id = :'pid'
		   AND actor_id   = :'actor'
		   AND revoked_at IS NULL;
		INSERT INTO api_keys (id, key_hash, team_id, project_id, actor_id, scopes)
		VALUES (gen_random_uuid()::text, :'hash', :'tid', :'pid', :'actor', '["*"]'::jsonb);
	`, "tid", teamID, "pid", projectID, "actor", actorID, "hash", keyHash)
	if err != nil {
		return nil, fmt.Errorf("rotate key: %w", err)
	}

	return map[string]string{
		"api_key":    rawKey,
		"team_id":    teamID,
		"project_id": projectID,
		"actor_id":   actorID,
	}, nil
}

// runPSQL launches the one-shot psql container, pipes `sql` on stdin, and
// returns the captured stdout (trimmed). Variadic `kv` pairs are passed as
// `-v name=value` so the SQL can reference them via `:'name'` interpolation
// — which is psql's quoted-literal substitution. This is the same
// injection-safe pattern the bash impl uses.
//
// stderr goes to os.Stderr unfiltered so psql errors surface to the user
// (we don't try to interpret psql diagnostics).
func (b *ClaudeMemBootstrap) runPSQL(ctx context.Context, image, dsn, sql string, kv ...string) (string, error) {
	if len(kv)%2 != 0 {
		return "", fmt.Errorf("runPSQL: variadic kv pairs must be even, got %d", len(kv))
	}

	// psql args:
	//   -tAq                 → tuples-only, unaligned, quiet (no command tags)
	//   -v ON_ERROR_STOP=1   → abort on first SQL error (otherwise psql
	//                          would chug through and exit 0)
	//   -v k=v ...           → user-supplied variable bindings
	//   -f -                 → read SQL from stdin (more reliable than `-c`
	//                          for :'name' interpolation, especially across
	//                          psql versions)
	cmd := []string{"psql", dsn, "-tAq", "-v", "ON_ERROR_STOP=1"}
	for i := 0; i < len(kv); i += 2 {
		cmd = append(cmd, "-v", kv[i]+"="+kv[i+1])
	}
	cmd = append(cmd, "-f", "-")

	var stdout bytes.Buffer
	err := b.Docker.Run(ctx, docker.RunSpec{
		Image:    image,
		Remove:   true,
		AddHosts: []string{"host.docker.internal:host-gateway"},
		Cmd:      cmd,
		Stdin:    strings.NewReader(sql),
		Stdout:   &stdout,
		Stderr:   os.Stderr,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// mintClaudeMemKey produces a fresh `cmem_<48-hex>` plaintext key. The 48
// hex chars come from 24 random bytes — same width as the bash impl
// (`openssl rand -hex 24`).
//
// `rng` is optional; nil means use crypto/rand.Reader.
func mintClaudeMemKey(rng io.Reader) (string, error) {
	if rng == nil {
		rng = rand.Reader
	}
	buf := make([]byte, claudeMemKeyRandomBytes)
	if _, err := io.ReadFull(rng, buf); err != nil {
		return "", err
	}
	return claudeMemKeyPrefix + hex.EncodeToString(buf), nil
}

// sha256Hex is the SHA-256 hash of s, hex-encoded, no trailing newline.
// Matches the server's hash-on-write convention (and the bash impl's
// `sha256sum | awk '{print $1}'`).
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// rewriteForOneshot rewrites localhost / 127.0.0.1 hosts in the DSN to
// host.docker.internal so the one-shot postgres container can reach the
// host's daemon. Preserves the userinfo and the trailing port/path
// separator character.
func rewriteForOneshot(url string) string {
	return localhostRewrite.ReplaceAllString(url, "//${1}host.docker.internal${3}")
}

// rewriteForHostProbe is the inverse of rewriteForOneshot: it rewrites
// host.docker.internal back to 127.0.0.1 so probes running on the host
// (where the Docker DNS name doesn't resolve) can reach the same endpoint
// that the workspace container would. URLs that don't reference
// host.docker.internal pass through untouched.
func rewriteForHostProbe(url string) string {
	return hostInternalRewrite.ReplaceAllString(url, "//${1}127.0.0.1${2}")
}

// cmDefault returns s if non-empty, else def. Small helper.
func cmDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
