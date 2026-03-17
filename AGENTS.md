# Agent Instructions

This project uses **br** (beads-polis v0.1.0) for issue tracking. Run `br ready` to get started.

```
BEADS: Use `POLIS_ACTOR=<agent-name> BEADS_DIR=/home/polis/projects/.beads br <command>`
Source of truth: /home/polis/projects/.beads/events.jsonl
Index (derived): /home/polis/projects/.beads/index.db
```

## Quick Reference

```bash
br ready              # Find available work
br show <id>          # View issue details
br claim <id>         # Claim work
br close <id>         # Complete work
```

## Subcommands

### catalog-check

Validates cross-runtime skill contracts from a registry.yaml file. Each entry declares binaries, source files, target files, and verify commands that must be present and healthy.

**Usage:**
```bash
gate catalog-check [--registry <path>] [--json]
```

**Flags:**
- `--registry <path>` — Override the registry file path (default: `/home/polis/tools/gate/registry.yaml`)
- `--json` — Output results as machine-readable JSON instead of a formatted table

**Output:** Each registry entry gets one of three statuses:
- **PASS** — Binary on PATH, verify commands succeed, source/target files exist and are non-empty
- **STALE** — Binary works but source or target files are missing/empty
- **BROKEN** — Binary missing from PATH or verify command failed

**Exit codes:**
- `0` — All entries pass
- `1` — Any entry is STALE or BROKEN

BROKEN entries automatically create beads (via `br`) for tracking.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
