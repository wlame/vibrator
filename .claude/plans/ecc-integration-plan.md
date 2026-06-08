# Plan: Adopting ECC ("Everything Claude Code") into vibrator

_Date: 2026-06-02 · Branch: pivot_

## STATUS: Phases 0–5 COMPLETE (2026-06-02)

Commits on `pivot`: 415dc98 (P1) · aa3bacf (P2) · eba6b3a (P3) · e4c4dae (P4) ·
c58f71b (P5). 17 `ecc-*` extensions shipped (6 claude-code, 5 codex, 6 opencode),
wizard detail pane added, README documented, pi gap documented. All Go tests +
vet pass; install snippets verified end-to-end on the host (no Docker available);
Dockerfile generation verified via `build-dockerfile`. Remaining/optional:
Phase 6 (aura as an integration), and a real `docker build` smoke test once an
environment with Docker is available.

## 1. What ECC is (verified from the repo, not the marketing copy)

ECC (`affaan-m/ECC`, **v2.0.0-rc.1**, MIT) is a **cross-harness content bundle +
installer** for AI coding agents. It is *not* a running service — it is a large
library of markdown definitions plus a Node installer that copies the right
subset into each harness's native config directory.

Real inventory (counted from a fresh clone, correcting the inflated WebFetch summary):

| Component        | Count | What it is |
|------------------|-------|------------|
| `agents/`        | 63    | Subagent definitions (frontmatter: name/description/tools/model + a "prompt-defense baseline") |
| `skills/`        | 404 files (~249 skills) | Workflow definitions (`SKILL.md` + supporting files). The canonical surface. |
| `commands/`      | 79    | Legacy slash-command shims (being migrated to skills) |
| `rules/`         | 115 files | `common/` (always-on) + per-language packs (ts, python, go, …) |
| `hooks/`         | 4     | Event automations (PreToolUse/PostToolUse/Stop/etc.) |
| `mcp-configs/`   | 1     | MCP server catalogue defaults |
| `integrations/aura` | — | An "AgentShield"-style security adapter (Python) |
| `scripts/`       | 168   | Node/bash installer + per-harness sync tooling |

**Install model.** A manifest-driven Node installer (`scripts/install-apply.js`)
reads `manifests/install-{profiles,components,modules}.json` (validated by JSON
schemas) and writes into harness-native dirs:

- **claude-code** → `~/.claude/{skills,agents,commands,rules,hooks}` + `settings.json` hooks
- **codex** → `scripts/sync-ecc-to-codex.sh`: merges `AGENTS.md` (marker-based),
  `commands/*` → `~/.codex/prompts/`, agents → `~/.codex/agents`, MCP → `config.toml`
- **opencode** → `scripts/build-opencode.js`: TS-compiles `.opencode/dist`
- also writes `.cursor/`, `.zed/`, `~/.qwen` (harnesses vibrator doesn't ship)

**Profiles** (orthogonal to vibrator's own image profiles): `minimal`, `core`,
`developer`, `security`, `research`, `full` — each is a set of *modules*
(`rules-core`, `agents-core`, `commands-core`, `hooks-runtime`,
`framework-language`, `security`, `research-apis`, …).

**Harness coverage vs. vibrator.** ECC supports claude-code, codex, opencode
(and cursor/zed/copilot/qwen). It has **no `pi` adapter** — pi is vibrator-only.
So "all harnesses ECC supports" ∩ "vibrator's harnesses" = **claude-code, codex,
opencode** (3 of 4).

## 1b. Phase 0 findings (2026-06-02) — COMPLETE

Probed the real installer (`scripts/install-apply.js`) at the pinned SHA. Results
**simplify the plan** materially:

- **Pin SHA:** `99baa8250096f2d295583572399a5c9aba2ce312` (2026-06-02, commit #2119).
  Default branch HEAD; an rc line, so bump deliberately.
- **One unified installer covers all targets.** `install-apply.js --target <T>` accepts
  `claude, claude-project, cursor, codex, gemini, opencode, qwen, zed, …` — including
  **claude, codex, opencode**. We do **not** need `sync-ecc-to-codex.sh` /
  `build-opencode.js` as separate vectors for claude/codex. One command shape:
  `node scripts/install-apply.js --target <T> --profile <P>`.
- **Flag-driven, non-interactive.** `--profile`, `--target`, `--with/--without`,
  `--modules`, `--skills`, `--dry-run`, `--json`. No readline/inquirer prompts →
  container-safe. `--dry-run --json` gives a machine-readable plan → perfect for
  vibrator **golden tests** (assert the plan without copying).
- **Home-dir installs, no host-global side effects** in the claude path: claude→
  `~/.claude/` (managed under `rules/ecc/` + `skills/ecc/` so user content isn't
  clobbered; writes `install-state.json`), codex→`~/.codex/.agents/`,
  opencode→`~/.opencode/`. No sudo, no global git-config in `install-apply.js`
  itself. (The global-git-hook step lives only in the optional codex *sync* script,
  which we're not using.)
- **npm deps are tiny:** 3 runtime deps (`ajv`, `@iarna/toml`, `sql.js`), 7 packages,
  ~22M node_modules, ~1s install. Build-time `npm install --omit=dev` is cheap.
- **Installed footprint is image-friendly** (claude target, measured):

  | profile | total | skills | agents | rules | commands | hooks |
  |---------|-------|--------|--------|-------|----------|-------|
  | core      | 4.2M | 544K | 504K | 628K | 492K | 80K |
  | developer | 5.5M | 1.8M | 504K | 628K | 492K | 80K |
  | full      | 7.4M | 3.6M | 504K | 628K | 492K | 80K |

  (The 77M clone is `.git` + `assets/` images — none of it is installed. Drop the
  clone + node_modules in the same RUN layer.)
- **Per-target nuance:**
  - **claude** — full module set (developer = 593 ops, 9 modules). Cleanest.
  - **codex** — installer **auto-reduces** to applicable modules (developer →
    keeps agents-core/platform-configs/database/workflow-quality; skips rules/
    commands/hooks/framework/orchestration → 184 ops). Valid, smaller. No extra work.
  - **opencode** — **requires a pre-build:** `npm run build:opencode` (needs dev
    deps incl. TypeScript) to produce `.opencode/dist/{index.js,plugins,tools}`
    *before* `--target opencode`. So opencode's snippet = full `npm install` +
    `npm run build:opencode` + installer. Slightly heavier; isolated to that file.

### Wizard "conscious choice" note (your requirement)
The picker (`internal/wizard/extensions_picker.go`) currently renders **one line per
entry** — `Name — Description [Category] badges` — and a keybindings footer. There is
**no detail pane** for the focused row. To let users consciously opt into ECC knowing
*what it is and does*, two layers:
1. **Now (free):** write a punchy one-line `description:` in each `ecc-*` entry, e.g.
   *"Everything-Claude-Code: 63 agents + ~249 skills + rules/hooks into ~/.claude —
   powerful but heavy agent context; opt-in"*. The full body is shown by
   `vibrate extensions show ecc-developer`.
2. **Phase 5 (small picker enhancement):** add a **focused-entry detail line/pane**
   below the list that shows the focused entry's longer note (a new optional
   `note:`/`summary` frontmatter field, or first paragraph of `Body`). Benefits every
   extension, not just ECC. This directly satisfies "show a note … to consciously choose."

## 2. How it would be used inside a vibrator container

A user picks an ECC profile when configuring a workspace; at image-build time
vibrator clones ECC (pinned SHA) and runs ECC's own installer for the chosen
harness + profile, populating `~/.claude` / `~/.codex` / `.opencode`. The agent
inside the container then has ECC's skills/agents/rules/hooks available natively.

## 3. Where ECC fits in vibrator's architecture

vibrator has three extension surfaces:

1. **`internal/feature`** — apt/runtime deps (node, python, …), topological resolver.
2. **`internal/extensions`** — per-harness `extensions/<harness>/<id>.md` with
   frontmatter (`kind` ∈ plugin|skill|mcp|subagent|tool, `category`, `deps.features`,
   `runtime_needs`, `install:` shell snippet) + docs body. The `install:` snippet is
   rendered into **Dockerfile Stage 4** as a heredoc `RUN`, executed as the
   unprivileged user (so it can write `~/.claude`, etc.). **Build-time, static.**
3. **`internal/integration`** — host-side, *stateful* services (serena, claudemem)
   with `Runtimes`, `ProbeFn`, per-harness `Wiring`, `LaunchChecks`. Wired at
   **runtime** by `claude-exec.sh` from `/etc/vibrator/integrations.json`.

**Decision: ECC is an _extension_, not an _integration._** ECC is static content
installed into the home dir at build time — there is no host server to probe, no
runtime MCP wiring, no per-workspace credential minting. That is exactly what the
extension surface (build-time `install:` snippet) models. The integration surface
(probes/wiring) would be dead weight.

`aura` (ECC's security adapter) is the one piece that *could* later become an
integration if it runs as a service — out of scope for v1.

## 4. Recommended design

### 4.1 New extension files (the deliverable)

Create ECC extension entries per harness:

```
extensions/claude-code/ecc-<profile>.md
extensions/codex/ecc-<profile>.md
extensions/opencode/ecc-<profile>.md
```

Each `install:` snippet (validated against the real installer in Phase 0). The
**claude/codex** form is identical bar the `--target`; **opencode** adds a pre-build:

```sh
# claude-code / codex — unified installer, prod deps only.
ECC_REF=99baa8250096f2d295583572399a5c9aba2ce312   # pinned; bump deliberately
git clone --filter=blob:none https://github.com/affaan-m/ECC.git /tmp/ecc
git -C /tmp/ecc checkout "$ECC_REF"
cd /tmp/ecc && npm install --no-audit --no-fund --omit=dev --loglevel=error
node scripts/install-apply.js --target claude  --profile developer   # → ~/.claude
#   codex variant:  --target codex                                   # → ~/.codex
cd / && rm -rf /tmp/ecc                                              # drop clone+node_modules
```

```sh
# opencode — needs the compiled plugin payload first (TypeScript dev dep required).
ECC_REF=99baa8250096f2d295583572399a5c9aba2ce312
git clone --filter=blob:none https://github.com/affaan-m/ECC.git /tmp/ecc
git -C /tmp/ecc checkout "$ECC_REF"
cd /tmp/ecc && npm install --no-audit --no-fund --loglevel=error   # incl. dev (tsc)
npm run build:opencode                                             # → .opencode/dist
node scripts/install-apply.js --target opencode --profile developer # → ~/.opencode
cd / && rm -rf /tmp/ecc
```

Verify the install plan deterministically with `--dry-run --json` (drives the
golden tests).

Frontmatter highlights:
- `kind: plugin` (it bundles skills+agents+hooks+rules)
- `category: harness-specific` (or a new `agent-framework` category — see Q below)
- `deps: { features: [node, git] }`
- `runtime_needs: { outbound_net: true }` for any ECC MCP/research skills; otherwise `local_only: true`
- `size_mb:` set realistically per profile

### 4.2 Profile granularity — the key design fork (see questions)

Two viable shapes:

- **(A) Profile-level IDs** — `ecc-core`, `ecc-developer`, `ecc-security`,
  `ecc-research`, `ecc-full`. One toggle = one ECC profile. Simple; mirrors ECC's
  own model 1:1; wizard stays clean. **Recommended.**
- **(B) Modular IDs** — `ecc-rules`, `ecc-agents`, `ecc-skills`, `ecc-security`, …
  composable but requires reimplementing ECC's module-resolution in our snippets;
  more surface, more drift risk.

### 4.3 Pi harness

ECC has no pi adapter. Options: **skip pi** (document it), or write a thin
vibrator-side pi adapter that copies skills/rules into pi's config layout. v1:
skip, leave a `TODO` extension stub.

## 5. Build sequence

- **Phase 0 — Verify ECC CLI contract — ✅ DONE (2026-06-02).** See §1b. Unified
  `install-apply.js --target <T> --profile <P>` covers claude/codex/opencode;
  non-interactive; home-dir installs; 4–7M footprint; SHA pinned; opencode needs a
  pre-build. No blockers.
- **Phase 1 — claude-code, `ecc-developer` only.** One extension file end-to-end:
  `vibrate build --harness=claude-code --extensions=ecc-developer`, confirm
  `~/.claude/skills` etc. populate. Add `extension_install_test.go` golden coverage.
- **Phase 2 — Remaining claude-code profiles** (`ecc-core/security/research/full/minimal`).
- **Phase 3 — codex** via `sync-ecc-to-codex.sh`; verify `AGENTS.md` merge + prompts.
- **Phase 4 — opencode** via `build-opencode.js`; verify `.opencode/dist`.
- **Phase 5 — Docs + wizard.** README extension table, `vibrate extensions show`
  bodies, wizard category grouping. Optional: pi stub.
- **Phase 6 (optional, later) — `aura` security integration** as a true
  `internal/integration` if/when it runs as a service.

## 6. Risks & mitigations

- **Image/context bloat** — 404 skills is a lot of agent context. → Default to a
  mid profile (`developer`); never auto-enable; document context cost; prefer
  ECC's own profile slicing over installing `full`.
- **Build-time network + npm install** — ECC installer needs node_modules. →
  Already how vibrator builds; pin SHA; `--filter=blob:none` + drop `.git`.
- **Upstream is rc/volatile** — `2.0.0-rc.1`, migrating commands→skills. → Pin a
  reviewed SHA; bump deliberately; never track a moving branch.
- **Installer assumes interactive/global host** — it backs up `~/.codex`, installs
  git hooks globally. → Run in the throwaway container home only; verify
  `--dry-run`/non-interactive paths in Phase 0; strip host-git-hook steps if they
  reach outside the workspace.
- **License/provenance** — MIT, fine; preserve ECC LICENSE/attribution in the
  installed tree.

## 7. Resolved decisions (2026-06-02)

1. **Granularity → profile-level IDs.** Ship `ecc-core`, `ecc-developer`,
   `ecc-security`, `ecc-research`, `ecc-full` (and `ecc-minimal` for claude-code).
   One toggle = one ECC profile; ECC's own installer does module resolution.
2. **Default/headline profile → `developer`.** Wizard/docs present `ecc-developer`
   as the recommended entry point; never auto-enabled.
3. **Pi → skip with a stub.** Document that ECC has no pi adapter; leave a TODO
   `extensions/pi/ecc.md` stub. Ship claude-code, codex, opencode in v1.
4. **Sourcing → git-clone at build, pinned SHA.** `install:` snippet clones ECC
   at a reviewed commit, `--filter=blob:none`, drops `.git`. No embed-FS vendoring.

### Pin to record
Capture the exact ECC commit SHA in Phase 0 and hard-code it as `ECC_REF` in every
`ecc-*` extension snippet. Bump deliberately via a single find-and-replace; never
track a moving branch (upstream is `2.0.0-rc.1` and actively migrating).
