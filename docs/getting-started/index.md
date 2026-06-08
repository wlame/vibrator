# Getting started

New to Vibrator? Start here.

<div class="grid cards" markdown>

-   :material-download:{ .lg .middle } **[Installation](installation.md)**

    ---

    Requirements, the `just install` flow, manual installation, and shell completion.

-   :material-rocket-launch-outline:{ .lg .middle } **[Quick start](quickstart.md)**

    ---

    Your first run, the setup wizard, skipping the wizard with flags, and re-runs.

-   :material-school-outline:{ .lg .middle } **[Core concepts](concepts.md)**

    ---

    The mental model: workspaces, variants, harnesses, profiles, features, extensions,
    and integrations.

</div>

## The shortest possible path

```bash
just install            # build + install the binary, alias, and completion
cd ~/my-project
vibrate                 # answer the wizard once; the agent drops you in
```

From then on, `vibrate` in that directory reuses the container and re-enters instantly.
Everything else in this documentation explains how to shape what gets built and how the
container behaves at runtime.
