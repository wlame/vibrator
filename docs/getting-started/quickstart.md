# Quick start

This page walks through your first `vibrate` run end to end: the wizard, what gets
written to `.vb`, how to skip the wizard entirely, and how re-runs behave.

## 1. Run `vibrate` in a project

```bash
cd ~/my-project
vibrate          # or `vb` — same binary
```

With no `.vb` file present, `vibrate` launches the **setup wizard**.

## 2. Answer the wizard

The [wizard](../reference/commands/wizard.md) asks only for what it needs, in order.
Steps you've already answered via flags or an existing `.vb` are skipped automatically:

| Step | Choices | Default |
|------|---------|---------|
| **Harness** | `claude-code`, `codex`, `opencode`, `pi` | — (required) |
| **Profile** | `minimal`, `backend`, `frontend`, `full` | `full` |
| **Shell** | `bash`, `zsh`, `fish` | `zsh` |
| **LLM provider** | shown only for provider-agnostic harnesses (Codex, OpenCode, Pi) | — |
| **Extensions** | multi-select from the harness's [catalogue](../guides/extensions.md) | per-entry defaults |
| **Serena hosting** | `auto`, `host`, `local`, `off` (Claude Code) | `auto` |

When you finish, the wizard prints a summary and the **equivalent command** — the exact
flags that reproduce your choices without the wizard next time. Copy it into a script if
you like.

## 3. The image builds and the agent starts

`vibrate` then:

1. Writes your choices to a [`.vb` file](../guides/configuration.md) and adds it to
   `.gitignore` (if one exists).
2. Generates a [Dockerfile](../lifecycle/build.md) for your spec and builds the image.
   This is the slow step — once per spec.
3. Creates the container, mounts your workspace, [forwards your
   credentials](../lifecycle/startup.md), and execs the [harness](../guides/harnesses.md)'s CLI.

You land inside the container, in your project directory, with a banner showing the
harness, auth status, profile, and tools available.

```
+--------------------------------------------------------------+
|  Vibrator — AI coding agent sandbox                          |
+--------------------------------------------------------------+

claude:       1.x.x
auth:         OAuth token
profile:      full
tools:        python,go,node,playwright,gh,postgres-client,...

workspace:    /Users/you/my-project
```

## 4. Subsequent runs are instant

Run `vibrate` again in the same workspace and it skips the wizard and the build —
it reuses the existing container (or starts it if stopped) and re-enters:

```bash
vibrate          # reuse container, jump straight in
```

When you exit the agent, the container stays around for next time. It's recreated
only when your spec changes or you ask for a [rebuild](../reference/commands/launch.md#options).

## Skip the wizard entirely

Pass everything as flags. `--no-wizard` makes missing required fields a hard error
instead of prompting:

```bash
vibrate --no-wizard \
        --harness=claude-code \
        --profile=full \
        --shell=zsh
```

Add and remove individual [features](../reference/features.md) on top of the profile,
and select [extensions](../guides/extensions.md):

```bash
vibrate --harness=claude-code \
        --profile=backend \
        --with=node,playwright \
        --no=ralphex \
        --extensions=context7,ecc-developer
```

To try a combination once without writing it to `.vb`, add `--no-save`.

## Get a shell instead of the agent

Everything about `vibrate run` applies to [`vibrate shell`](../reference/commands/launch.md#vibrate-shell)
— except the final step launches your shell instead of the harness CLI. Handy for
debugging, inspecting config, or running one-off commands:

```bash
vibrate shell
```

## Common follow-ups

- **Change your setup later** → [`vibrate reconfigure`](../reference/commands/wizard.md#vibrate-reconfigure)
  re-runs the wizard and rebuilds, preserving your credentials.
- **Update the agent CLI** → [`vibrate update`](../reference/commands/update.md) upgrades
  the harness in place without a full rebuild.
- **Authenticate** → if the banner says `auth: not configured`, see
  [Authentication](../guides/authentication.md) (including `vibrate --login`).
- **List what you've built** → [`vibrate variants list`](../reference/commands/variants.md).

## Next steps

- [Core concepts](concepts.md) — understand workspaces, variants, and the build pipeline.
- [The `.vb` file](../guides/configuration.md) — what gets saved and how to hand-edit it.

## Related pages

- [The wizard](../reference/commands/wizard.md) — every step of the setup form.
- [Profiles & features](../guides/profiles-and-features.md) — shaping what gets built.
- [Troubleshooting](../troubleshooting.md) — fixes if the first run misbehaves.
