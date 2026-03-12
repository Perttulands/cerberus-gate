# gate — Quality Gate for Polis

## What It Is

Gate protects main. Before code lands, `gate` runs quality checks and produces a verdict: pass, fail, or needs review. Every verdict is recorded as a bead so the city remembers what was checked and why.

Gate is simple and focused. It does not learn, does not orchestrate, does not decide what to do with a failure. It checks and reports. The caller (`work`, a human, a script) decides what happens next.

## CLI

```
gate check <repo-path>                    # run all gates, default level
gate check <repo-path> --level quick      # fast pass: tests + lint only
gate check <repo-path> --level deep       # thorough: full truthsayer + ubs scans
gate check <repo-path> --json             # machine-readable verdict

gate history <repo-path>                  # past verdicts for this repo
gate history --citizen mercury            # all verdicts for a citizen
```

## What It Checks

Tiered by level:

**Quick** (seconds):
- Test suite passes (auto-detects: go test, npm test, pytest, cargo test)
- Linter passes (auto-detects: go vet, eslint, ruff, shellcheck)

**Standard** (default, ~30s):
- Everything in quick
- Truthsayer scan — zero critical findings
- UBS scan — build health check

**Deep** (~2min):
- Everything in standard
- Risk scoring — file classification, change size, sensitive path detection
- Area fragility — have previous merges to this area failed?

## The Verdict

```json
{
  "pass": true,
  "level": "standard",
  "citizen": "zeus",
  "repo": "centurion",
  "gates": [
    {"name": "tests", "pass": true, "duration_ms": 4200},
    {"name": "lint", "pass": true, "duration_ms": 800},
    {"name": "truthsayer", "pass": true, "findings": 0, "duration_ms": 3100},
    {"name": "ubs", "pass": true, "duration_ms": 1200}
  ],
  "exit_code": 0,
  "bead": "gate-f3a2bc"
}
```

Exit codes: 0 = pass, 1 = fail, 2 = needs human review.

## Bead Recording

Every verdict creates a bead:
```
br create "gate check: pass" -t chore -a $POLIS_CITIZEN -l "tool:gate,status:pass,repo:centurion,level:standard"
```

This means `gate history` is just `br list` filtered by gate type — the history is in beads, not a separate database.

## Technical

- **Language:** Go
- **Dependencies:** truthsayer (optional), ubs (optional), br (optional)
- **Auto-detection:** scans repo for test runners, linters, configs
- **Zero-config:** works on any repo with sensible defaults
- **Config:** optional `gate.toml` in repo root for overrides

## What It Does NOT Do

- Merge branches (the caller decides what to do with the verdict)
- Learn from history (learning-loop does that)
- Capture traces (work does that)
- Fix issues (just reports them)

## Success

- Gates a real merge in under 60 seconds (quick mode)
- Auto-detects test framework in Go, Node, Python, Rust, bash projects
- Verdicts recorded as beads that tell the full story
- `gate check --json` output is parseable by `work` without transformation
- Works standalone — useful even without the rest of Polis
