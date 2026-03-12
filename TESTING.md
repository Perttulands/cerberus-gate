# Testing — gate

Quality gate CLI for Polis city tools.

## Rubric Scores

| Dimension | Before | After | Notes |
|---|---|---|---|
| E2E Realism | 2 | 3 | Added E2E tests for `runCheck` and `runCity` through full execution path |
| Unit Test Behaviour Focus | 3 | 4 | All major output functions now tested; formatCheckDescription, printPretty, printPrettyCity covered |
| Edge Case & Error Path Coverage | 3 | 3 | Same — existing edge cases were already solid; new tests add happy-path depth, not new error branches |
| Test Isolation & Reliability | 4 | 4 | New tests follow same patterns: t.TempDir(), os.Pipe(), no shared state |
| Regression Value | 3 | 4 | Breaking output formatting, CLI execution, command execution, or bead recording now caught |
| **Total** | **15 (C)** | **18 (B)** | |

## Coverage

| Package | Before | After |
|---|---|---|
| cmd/gate | 55.5% | 84.8% |
| internal/bead | 86.3% | 96.1% |
| internal/city | 89.6% | 89.6% |
| internal/gates | 88.1% | 97.6% |
| internal/pipeline | 94.3% | 94.3% |
| internal/verdict | 100% | 100% |
| **Overall** | **80.4%** | **91.0%** |

## Honest Assessment

### Strengths
- Parser tests (truthsayer, UBS) are thorough with JSON edge cases, banner prefixes, malformed input fallbacks
- City checks well tested with real git repos, symlinks, timeout, path traversal
- Good use of dependency injection (runCmdFunc, lookPath) for isolation
- Pipeline integration tests use real Go compilation — catches real breakage
- Table-driven tests throughout, clear test naming

### Remaining Gaps
- `main()` itself untestable (calls os.Exit) — acceptable, tested via `run()`
- `runHistory` not E2E tested (requires `br` binary) — only flag error paths covered
- `gitUserName` at 75% — untested error branch (git not available)
- `checkSplit` at 65.8% — glob match failure paths in city not fully exercised
- `loadConfig` at 79.2% — some TOML parse error branches uncovered
- `hasESLint` at 81.8% — JSON parse error branch uncovered
- No fuzz testing on parsers
- E2E tests add ~7s to suite due to real `go test`/`go vet` execution

## Phase 1 D2 Contract Inventory

- `gate -> truthsayer`: binary-level contract now pinned by a CLI E2E test that verifies `truthsayer scan . --format json` and JSON findings propagation.
- `gate -> ubs`: binary-level contract now pinned by the same CLI E2E test for standard-level diff invocation `ubs --diff --format=json .`; diff fallback remains covered in `internal/gates/gates_test.go`.
- `orbit -> gate health`: binary-level JSON/exit contract is covered for a failing repo, asserting `gate check --level quick --json` exits `1` while still returning machine-readable verdict output.

### What Would Get This to A (22+)
1. Fuzz testing for parseTruthsayerOutput and parseUBSOutput
2. E2E test for `gate history` with a mocked `br` binary
3. Cover checkSplit glob failure paths with a walkdir error injection
4. Test loadConfig with every TOML parse error variant
5. Integration test that exercises standard/deep levels through the CLI (not just pipeline)

## Test Architecture

```
cmd/gate/main_test.go
  - Argument parsing: flag errors, missing repo, unknown command
  - E2E: runCheck passing/failing Go project (JSON + pretty output)
  - Binary contract: real `gate` executable with fake `truthsayer`/`ubs` binaries and JSON health exit contract
  - E2E: runCity with real git repo + city.toml
  - Output formatting: printPretty (pass/fail/skip/bead), printPrettyCity (pass/warn/fail/bead)

internal/bead/record_test.go
  - Record/RecordCity with mocked br (pass/fail verdicts)
  - Graceful degradation when br unavailable
  - formatCheckDescription and formatCityDescription output
  - normalizeLabels sorting
  - Citizen assignee logic (unknown skips -a flag)
  - createWithBR error handling

internal/city/city_test.go + helpers_test.go
  - Full city check integration with temp git repos
  - Boundary check with .gitignore semantics
  - Standalone check with timeout
  - Config hooks validation (all fallback types)
  - Split check with type mismatches and symlinks
  - Path normalization (traversal, glob, backslash)
  - Glob pattern matching (**, *, ?, brackets)

internal/gates/
  gates_test.go: RunTests/RunLint/RunTruthsayer/RunUBS with mocked commands
  detect_test.go: Language/framework detection (Go, Node, Python, Rust, Bats, Shell)
  exec_test.go: Real command execution (success, failure, not found, timeout, stderr, dir)
  truthsayer_test.go: JSON/text output parsing with edge cases
  ubs_test.go: JSON/text output parsing with edge cases

internal/pipeline/
  pipeline_test.go: Level validation, gate composition per level
  integration_test.go: Real Go projects (pass/fail/vet), JSON roundtrip

internal/verdict/
  verdict_test.go: ComputeScore (all/some/none pass, skipped), TimedRun
```

## Changelog

### 2026-02-28 — Agent: ares
- Added: E2E tests for `runCheck` (passing project, failing project, pretty output)
- Added: E2E test for `runCity` (JSON output with real git repo)
- Added: Output formatting tests for `printPretty` (pass, fail with detail, skipped gate, bead ID)
- Added: Output formatting tests for `printPrettyCity` (pass, warn, fail, bead ID)
- Added: `formatCheckDescription` tests (fail verdict with mixed gates, pass verdict)
- Added: `normalizeLabels` sorting test
- Added: `Record` failure path tests (fail verdict title, unknown citizen, br crash)
- Added: `runCmdImpl` tests (success, non-zero exit, command not found, timeout, stderr capture, working dir)
- Coverage delta: 80.4% -> 91.0% (27 new tests covering CLI execution, output formatting, real command execution, bead recording)
