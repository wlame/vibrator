# Naming & labels

How Vibrator names images and containers, computes the variant fingerprint, sets the
container hostname, and labels everything for discovery. This is what makes
[isolation](../getting-started/concepts.md#variant) per workspace work.

## The fingerprint

Every build identity starts from an 8-character hex **fingerprint**: the first 8 chars of
SHA-256 over a canonical form of the spec.

The canonical form is:

```
harness=<lower>;shell=<lower>;features=<sorted-csv>;extensions=<sorted-csv>;user=<lower>
```

Properties:

- **Order-independent** — features and extensions are sorted before hashing, so the same
  logical selection always yields the same fingerprint regardless of input order.
- **Case-insensitive** for the small enums (harness, shell, user).
- **Profile is excluded** — it's just a label for a feature bundle. `--profile=full` and the
  implicit default resolve to the same features and therefore the same fingerprint.
- **Username is included** — the image's `USER` stage hard-codes the host UID/GID, so two
  users must get distinct images.
- A wholly empty spec hashes to the sentinel `00000000`.

## Image tag

```
vb-<harness>-<profile>-<user>-<fp8>:latest      # with a username
vb-<harness>-<profile>-<fp8>:latest             # without
```

Example: `vb-claude-code-full-you-a1b2c3d4:latest`.

- Harness, profile, and user are sanitized to safe tag characters (lowercase
  `[a-z0-9._-]`, no leading dot/dash).
- `:latest` is intentional — image versioning is per-workspace by fingerprint, not a
  floating tag.
- Override the tag for [`vibrate build`](commands/build.md) with `--tag`.

## Container name

```
vb-<workspace-basename>-<wsHash8>-<fp8>
```

Example: `vb-my-project-7f8e9a0b-a1b2c3d4`.

`wsHash8` is a hash of the absolute workspace path **plus the host UID**. That combination:

- disambiguates same-named projects in different paths (`~/work/foo` vs `~/play/foo`), and
- prevents multi-user collisions (alice's and bob's `~/dev/foo` get distinct containers,
  not one reused across UIDs).

## Hostname

```
vibrate-<sanitized-workspace-basename>
```

Example: `vibrate-my-project`. Set via `docker run --hostname`, it's what makes the shell
prompt visibly different from a host shell (`you@vibrate-my-project`). Sanitized per
RFC 1123 (letters, digits, hyphens; no leading/trailing hyphen; max 63 chars).

## Labels

### Image labels (baked at build time)

| Label | Value |
|-------|-------|
| `vibrator.version` | vibrator version |
| `vibrator.build_id` | build sentinel |
| `vibrator.harness` | harness ID |
| `vibrator.profile` | profile ID |
| `vibrator.shell` | shell |
| `vibrator.features` | comma-separated features |
| `vibrator.extensions` | comma-separated extensions |

### Container labels (set at run time)

| Label | Value |
|-------|-------|
| `vibrator.managed` | `true` |
| `vibrator.harness` | harness ID |
| `vibrator.workspace` | the fingerprint |
| `vibrator.path` | the workspace absolute path |

[`vibrate variants`](commands/variants.md) filters on `vibrator.managed=true` (containers)
and `vibrator.harness` (images) so it only ever lists or prunes Vibrator's own objects.

## Username derivation

The container username defaults to your host username, sanitized for Linux `useradd`:
lowercased, non-`[a-z0-9_-]` replaced with `_`, prefixed with `_` if it doesn't start with a
letter/underscore, truncated to 32 chars. It falls back to `vibrate` if detection fails or
you're root (UID 0). Override with `--username`.

## Related

- [Core concepts: variant](../getting-started/concepts.md#variant).
- [`vibrate variants`](commands/variants.md) — list/prune by label.
- [What happens on build](../lifecycle/build.md) — where labels are written.
