# gate

![Gate Banner](banner.png)

Gate is a quality checkpoint for code repositories. You point it at a repo, it runs your tests, linters, and security scans, and gives you a single verdict: pass or fail. If something breaks, it records the failure as a bead so you can track it. When you fix it, gate auto-closes the record.

It also validates whether a repo is ready to be installed into a running Polis city -- checking that boundaries are respected, hooks have sane fallbacks, and the thing actually works in isolation.

Gate exists because quality checks shouldn't be a ceremony you perform by hand. They should be a gate you walk through.

## Mythology

From `agents/hierophant/workspace/incoming/MYTHOLOGY-DRAFT.md`:

`gate` is the Centaur of Polis: the city gatekeeper.

- Sigil: horseshoe with a checkmark
- Shape: unmistakable centaur silhouette (nobody else has four legs)
- Armor: bronze chest plate engraved with four trials (`lint`, `test`, `scan`, `gate`)
- Tool: inspection hammer (he taps builds; hollow work fails)
- Authority: gold merge seal for worthy work, red brand for rejection

Narrative role:
- He walks the build sites.
- He runs the full gauntlet, including Truthsayer.
- Nothing enters the city without his gold seal.

This is why the command is legible (`gate`) while the identity is mythic (the Centaur).

## Installation

```sh
# From source
cd /path/to/gate
go build -o gate ./cmd/gate

# Or install to GOBIN
go install ./cmd/gate
```

## Quick Start

```sh
# Run standard quality check on current repo
gate check .

# Run with JSON output
gate check . --json

# Run quick check (tests + lint only)
gate check . --level quick

# Probe current repo health without creating a bead
gate health

# Validate city contract
gate city . --install-at /usr/local/share/myapp

# View gate history
gate history --repo myrepo --limit 10
```

## CLI Commands

### `gate check <repo-path> [flags]`

Run the quality gate pipeline and return a verdict.

```
gate check <repo-path> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--level quick\|standard\|deep` | `standard` | Gate level to run |
| `--json` | | Emit JSON verdict to stdout instead of colorized output |
| `--citizen <name>` | (see below) | Actor override for bead assignment |

**Gate levels:**
- `quick`: tests + all detected lint gates
- `standard`: quick + truthsayer + ubs scans
- `deep`: standard + full `truthsayer` and `ubs` scans

**Exit codes:**
- `0` — all non-skipped gates pass
- `1` — one or more gates fail
- `2` — reserved (`ExitReview`: warnings present but no hard failures); not currently emitted by the check pipeline

**Citizen resolution** (when `--citizen` is omitted): `POLIS_CITIZEN` env var → `git config user.name` in the target repo → literal `unknown`.

---

### `gate health [repo-path] [flags]`

Run the quick gate pipeline as a lightweight health probe without recording a bead.

```
gate health [repo-path] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `repo-path` | `.` | Repo to probe |
| `--json` | | Emit the quick verdict JSON to stdout |
| `--citizen <name>` | (see above) | Actor override used for the verdict metadata |

**Exit codes:**
- `0` — repo is healthy under the quick gate
- `1` — one or more quick gates failed

---

### `gate city <repo-path> [flags]`

Validate the city-readiness contract defined in the repo's `city.toml`.

```
gate city <repo-path> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--install-at <path>` | | Install root for split and fallback=fail hook checks |
| `--skip-standalone` | | Skip the standalone check (status becomes `skip`) |
| `--standalone-timeout <duration>` | `120s` | Timeout for the standalone check command |
| `--json` | | Emit JSON city verdict to stdout instead of colorized output |
| `--citizen <name>` | (see above) | Actor override for bead assignment |

**City checks performed (in order):**
1. `boundary` — verifies all `polis_files` entries are ignored by git-native semantics
2. `standalone` — shallow-clones the repo and runs `standalone_check` in isolation
3. `config-hooks` — validates hook files are listed in `polis_files` and have valid fallback values
4. `split` — verifies polis files are present at `--install-at` path

**Exit codes:**
- `0` — all checks pass
- `1` — one or more checks fail
- `2` — no failures but one or more checks skipped (warn)
- `3` — invalid input: missing repo path, `city.toml` parse/schema errors, or path resolution failure

---

### `gate history [flags]`

Query previously recorded gate beads via `br search`. Requires `br` on PATH.

```
gate history [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo <name>` | | Filter by repo name label (alphanumeric, `.`, `_`, `-` only) |
| `--citizen <name>` | | Filter by assignee (same character restrictions as `--repo`) |
| `--limit N` | `20` | Maximum number of results to return (must be positive) |

Streams `br` stdout/stderr directly. Returns `br`'s exit code on failure.

---

## Auto-detected Test Frameworks

`gate check` detects and runs the first matching test suite:

| Language | Detection file | Command |
|----------|---------------|---------|
| Go | `go.mod` | `go test ./...` |
| Node | `package.json` | `npm test` |
| Python | `pyproject.toml` or `setup.py` | `pytest` |
| Rust | `Cargo.toml` | `cargo test` |
| Bats | `*.bats` in repo root | `bats .` |

If no test suite is detected, the `tests` gate passes with `no test suite detected`.

## Auto-detected Linters

All applicable linters run independently:

| Linter | Detection | Command |
|--------|-----------|---------|
| `go vet` | `go.mod` present | `go vet ./...` |
| `eslint` | `package.json` + eslint in deps | `npx eslint .` |
| `ruff` | `*.py` files, `src/`, `pyproject.toml`, or `setup.py` | `ruff check .` |
| `shellcheck` | `*.sh` files in repo root | `shellcheck <each-root-sh-file>` |

If no linters are detected, a single pass gate `lint` is emitted with `no linters detected`.

## Security Scan Gates (standard level and above)

- **truthsayer**: runs `truthsayer scan . --format json`. Skipped (pass) if binary absent.
- **ubs**: runs `ubs --format=json .`. In diff mode, retries full scan if diff scan fails. Skipped (pass) if binary absent.

---

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `POLIS_CITIZEN` | Default citizen identity for bead assignee and verdict field; used when `--citizen` is absent or blank |

The standalone check (`gate city`) runs in an isolated environment. Only these variables are passed through: `PATH`, `HOME`, `TMPDIR`, `LANG`, `LC_ALL`, `TERM`.

### `gate.toml` (consumed by `gate check`)

Optional. Place in the repo root to override auto-detected test, lint, and scan commands. When absent, gate falls back to auto-detection (see tables above). When present, only the fields you set are overridden; omitted fields still auto-detect.

```toml
[check]
# Override the test command (replaces auto-detected framework).
# Each element is an argv token — no shell expansion.
test = ["go", "test", "-count=1", "./..."]

# Override truthsayer scan command.
truthsayer    = ["truthsayer", "scan", ".", "--format", "json"]
truthsayer_ci = ["truthsayer", "scan", ".", "--format", "json", "--ci"]

# Override ubs scan command.
ubs      = ["ubs", "--format=json", "."]
ubs_diff = ["ubs", "--format=json", "--diff", "."]

# Override linters. Each entry needs a name and argv.
[[check.lint]]
name = "golangci-lint"
cmd  = ["golangci-lint", "run", "./..."]

[[check.lint]]
name = "shellcheck"
cmd  = ["shellcheck", "scripts/deploy.sh"]
```

### `city.toml` (consumed by `gate city`)

Place in the repo root. Required fields:

```toml
[city]
schema_version = 1                    # required, must be 1
polis_files = ["path/to/file", "dir/"] # relative paths; supports glob patterns; trailing / = directory
standalone_check = "go test ./..."    # shell command; empty string skips standalone check

[[hook]]
file = "path/to/hook-file"           # relative non-glob file path; must be in polis_files
fallback = "defaults"                # one of: defaults, fail, env:<VAR>
```

### Internal Gate Timeouts (defaults when not overridden by caller)

| Gate | Default timeout |
|------|----------------|
| Tests | `120s` |
| Lint | `60s` |
| Truthsayer | `60s` |
| UBS | `60s` |
| City standalone check | `120s` (configurable via `--standalone-timeout`) |

---

## Dependencies

### Required

| Tool | Purpose |
|------|---------|
| `git` | Repo validation, ignore checks, shallow clone for standalone, citizen fallback |

### Optional

| Tool | Purpose |
|------|---------|
| `br` | Bead creation, deduplication, auto-close, and `gate history` |
| `truthsayer` | Security/quality scan (standard and deep levels) |
| `ubs` | Unused/bad symbol scan (standard and deep levels) |

### Contextual (only if repo type requires)

`go`, `npm`, `pytest`, `cargo`, `bats`, `ruff`, `shellcheck`, `npx`, `bash`

### Go Module Dependencies

- `github.com/pelletier/go-toml/v2 v2.2.4` — `city.toml` parsing

---

## Current Status

✅ `gate check` — full pipeline works: test detection, lint detection, security scans, scored verdicts, JSON output
✅ `gate city` — boundary, standalone, hooks, and split checks all functional
✅ Bead integration — failure tracking with deduplication and auto-close on recovery
✅ `gate history` — queries prior gate records when `br` is available
⚠️ `deep` currently means full scanners only; no separate risk verdict is emitted until that check is real
⚠️ `gate history` requires `br` on PATH; unavailable otherwise
⚠️ `truthsayer` and `ubs` gates silently skip (pass) when their binaries are absent — gate won't tell you it's not scanning
⚠️ Exit code 2 (`ExitReview`) exists in the codebase but is not emitted by the check pipeline

---

## Part of Polis

Gate is one tool in a larger system. The others:

| Tool | Role | Repo |
|------|------|------|
| **ergon** | Work orchestration | [ergon-work-orchestration](https://github.com/Perttulands/ergon-work-orchestration) |
| **hermes** | Relay | [hermes-relay](https://github.com/Perttulands/hermes-relay) |
| **chiron** | Agent trainer | [chiron-trainer](https://github.com/Perttulands/chiron-trainer) |
| **learning-loop** | Learning loop | [learning-loop](https://github.com/Perttulands/learning-loop) |
| **senate** | Senate | [senate](https://github.com/Perttulands/senate) |
| **beads** | Bead system | [beads-polis](https://github.com/Perttulands/beads-polis) |
| **truthsayer** | Code scanner | [truthsayer](https://github.com/Perttulands/truthsayer) |
| **ubs** | Bug scanner | [ultimate_bug_scanner](https://github.com/Perttulands/ultimate_bug_scanner) |
| **oathkeeper** | Oath enforcement | [horkos-oathkeeper](https://github.com/Perttulands/horkos-oathkeeper) |
| **argus** | Watcher | [argus-watcher](https://github.com/Perttulands/argus-watcher) |
| **utils** | Shared utilities | [polis-utils](https://github.com/Perttulands/polis-utils) |
