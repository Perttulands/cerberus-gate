# gate

![Gate Banner](banner.png)

`gate` is the Centaur of Polis: the city gatekeeper.

It has two jobs:
- `gate check`: quality gate for repository changes
- `gate city`: city-readiness gate for safe installation and upgrades

## Mythology

From `agents/hierophant/workspace/incoming/MYTHOLOGY-DRAFT.md`:

The Centaur is the gatekeeper of Polis.

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

## Usage

```bash
gate check <repo-path> [--level quick|standard|deep] [--json] [--citizen <name>]
gate city <repo-path> [--install-at <path>] [--skip-standalone] [--standalone-timeout 120s] [--json]
gate history [--repo <name>] [--citizen <name>] [--limit N]
```

## City Contract

`gate city` reads `city.toml` and verifies:
- boundary declaration (`polis_files` are truly git-ignored)
- standalone functionality (clean clone check)
- config hooks and fallbacks
- split on disk at install location (`--install-at`)

See `PRD-city.md` for the prescriptive contract.

## Dependencies

Requires: `truthsayer` -- runs Truthsayer as the code-scanning gate head.
Optional: `ubs` -- enables deep bug scanning as a third gate head.
