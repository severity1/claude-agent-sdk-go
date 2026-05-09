---
description: Comprehensive code review using team-based specialized agents with self-coordination
allowed-tools: Read, Grep, Glob, Bash, Edit, Write, Task, WebFetch, TeamCreate, TeamDelete, TaskCreate, TaskUpdate, TaskList, TaskGet, SendMessage, AskUserQuestion, ToolSearch
---

# TDD Code Review (Team-Based)

Comprehensive code review for the Go SDK. Orchestrates multiple reviewers to analyze Go idioms, error handling, type design, test coverage, and Python SDK parity.

## Prerequisite: Load Deferred Tools

Team and task orchestration tools (`TeamCreate`, `TeamDelete`, `TaskCreate`, `TaskUpdate`, `TaskList`, `TaskGet`, `SendMessage`, `AskUserQuestion`) are deferred in this harness. Before entering Phase 1, load them via:

```
ToolSearch(query="select:TeamCreate,TeamDelete,TaskCreate,TaskUpdate,TaskList,TaskGet,SendMessage,AskUserQuestion", max_results=8)
```

Skip this for quick-mode runs.

## Quick Mode Exception

For focused reviews, use a single bare Agent call (model: "sonnet") with NO team:

- `--quick` — Only `grumpy-gopher`
- `--errors` — Only `pr-review-toolkit:silent-failure-hunter`
- `--types` — Only `pr-review-toolkit:type-design-analyzer`
- `--tests` — Only `pr-review-toolkit:pr-test-analyzer`
- `--parity` — `grumpy-gopher` with explicit instructions to cross-reference `docs/tracking/README.md` and `../claude-agent-sdk-python/` for Python SDK divergence

If `$ARGUMENTS` contains any of these flags, extract the flag, launch the single appropriate agent with the file list, present results, and stop. Do not create a team, do not load team tools.

---

## Review Scope Detection

Determine what code to review:

1. **User-specified files**: If user provided specific files or packages after the flag/arguments, use those.
2. **Unstaged changes**: `git diff --name-only`
3. **Staged changes**: `git diff --staged --name-only`
4. **Default**: Current package (`./...`)

```bash
git diff --name-only
git diff --staged --name-only
```

Store the file list. If no files found, report and exit cleanly (no team created).

---

## Reviewer Selection

Based on files identified, select reviewers:

| File Pattern | Reviewer |
|--------------|----------|
| `internal/control/*` | `grumpy-gopher`, `pr-review-toolkit:silent-failure-hunter` |
| `internal/parser/*` | `grumpy-gopher`, `pr-review-toolkit:silent-failure-hunter` |
| `internal/subprocess/*` | `grumpy-gopher`, `pr-review-toolkit:silent-failure-hunter` |
| `internal/cli/*` | `grumpy-gopher` |
| `internal/shared/*` | `grumpy-gopher`, `pr-review-toolkit:type-design-analyzer` |
| Root `*.go` (public API: `client.go`, `query.go`, `options.go`, `errors.go`, `transport.go`) | `grumpy-gopher`, `pr-review-toolkit:type-design-analyzer` |
| `*_test.go`, `*_bench_test.go` | `pr-review-toolkit:pr-test-analyzer` |
| `examples/*` | `grumpy-gopher` (informational only) |
| Any Go file | `grumpy-gopher`, `pr-review-toolkit:silent-failure-hunter` (always) |
| Scope spans 2+ packages | Add `pr-review-toolkit:code-reviewer` |
| Diff adds large docstrings/comment blocks | Add `pr-review-toolkit:comment-analyzer` |

**Always include** for any Go files: `grumpy-gopher`, `pr-review-toolkit:silent-failure-hunter`.

Deduplicate: if a reviewer would be selected multiple times, launch once with all matching files.

### Graceful Degradation

If a `pr-review-toolkit:*` reviewer is unavailable (plugin not installed), skip it with a warning in the summary and continue. Do not fail the review.

---

## Phase 1: Team Creation & Spawn

### Create Team

Generate a short ID from timestamp or random suffix:

```
TeamCreate(team_name="review-{short-id}", description="Code review for [file summary]")
```

### Create Review Tasks

One task per selected reviewer. No dependencies - all run in parallel.

Example:

```
Task 1: "Review: Go idioms, project conventions, Python SDK parity" (files: [all Go files])
Task 2: "Review: Error handling and silent failures" (files: internal/control/*, internal/parser/*, internal/subprocess/*)
Task 3: "Review: Type design for public API" (files: root *.go, internal/shared/*)
Task 4: "Review: Test coverage and patterns" (files: [test files])
```

Each task description includes: reviewer type, file list, focus areas.

### Spawn Reviewers

All reviewers use `model: "sonnet"`. Each gets this inline prompt:

```
You are a reviewer on team review-{short-id}.

Self-coordination loop:
1. Call TaskList for unblocked, unassigned review tasks
2. Claim lowest-ID matching task via TaskUpdate (set owner to your name)
3. Read task description via TaskGet for file list and focus areas
4. Execute review per your methodology
5. Send findings to lead via SendMessage in this format:
   - CRITICAL: [issues that must fix - bugs, security, data loss, parity breaks]
   - MAJOR: [should fix - significant quality issues]
   - MINOR: [nice to fix - style, minor improvements]
   - POSITIVE: [good patterns observed]
6. Mark task completed via TaskUpdate
7. Check TaskList for more work. If none: message lead "Review complete. Standing by."

Project-specific focus:
- Idiomatic Go (gofmt, context-first, fmt.Errorf with %w)
- Cyclomatic complexity under 15 (per gocyclo)
- No unreachable internal functions (per `make deadcode` — scoped to `internal/*` rooted at `./examples/...`); flag dead code in `internal/*` or public API symbols not wired through any internal path
- Thread-safe mocks; t.Helper() in test helpers
- Python SDK parity - cross-check ../claude-agent-sdk-python/ and docs/tracking/README.md. The goal is feature parity with the *observable behavior* of the Python SDK: wire format (JSON field names, constants, message shapes, CLI flags), public API surface, and semantics exposed to consumers. Parity on internal mechanics (helper return types, private struct layout, control flow shape) is not a goal in itself — since this is Go, prefer idiomatic Go and established Go best practices (nil-safety, context-first, error wrapping with %w, zero-value usability, small focused interfaces, gofmt/linter conformance) over mirroring Python internals. When a Go idiom and a Python internal shape conflict, choose the Go idiom and note the deliberate divergence in code or commit message.
- No unnecessary exports; interfaces small and focused
- Before recommending a change to an existing line, run `git log --oneline -20 -- <file>` and scan recent commits on this branch. If the current shape was introduced deliberately in a recent commit (commit message indicates intent, not a mechanical refactor), cite that commit SHA in your finding and frame it as "reconsider X because Y" rather than "fix X". Do not silently recommend reversing a recent deliberate change.

Restrictions:
- NEVER use TeamCreate, TeamDelete, or broadcast
- NEVER modify source code - review only
- Report ALL findings with file:line references
```

---

## Phase 2: Collect Results

Wait for all reviewer messages. As each arrives:

1. **Collect findings** from each reviewer message.
2. **Deduplicate** — same issue reported by multiple reviewers (keep the most detailed).
3. **Categorize by severity**:
   - **Critical**: Must fix (bugs, security, data loss, Python SDK parity breaks)
   - **Major**: Should fix (significant quality issues, missing tests for new behavior)
   - **Minor**: Nice to fix (style, minor improvements)

---

## Phase 3: Filter Tracked Issues

Before presenting, filter out issues tracked elsewhere:

### Check GitHub Issues

```bash
gh issue list --label phase-2 --state open --json number,title,body --limit 50
gh issue list --label parity --state open --json number,title,body --limit 50
```

### Check Python SDK Parity Tracker

Read `docs/tracking/README.md` — items listed as pending phases (Phase 2/3/4) are planned work, not review gaps.

### Filtering Criteria

**FILTER OUT** if:

1. **Tracked in GitHub Issues** — already planned. Note: "Tracked in #123".
2. **Tracked in `docs/tracking/README.md`** — scheduled Python SDK parity port.
3. **Example/demo code** (`examples/` directory) — unless misleading for library users.
4. **Documentation-only** — missing docs for planned features.
5. **Test infrastructure** — test helpers that ship with their feature.

**KEEP** if:

- Blocks current branch functionality.
- Code quality issue in core library code (`internal/*`, root `*.go`).
- Security or correctness issue regardless of location.
- Python SDK parity break in already-ported functionality.

### Present Filtered List

```
FILTERED ISSUES (addressed by other planned work):
- [Issue description] — Reason: Tracked in #123
- [Issue description] — Reason: Phase 2 item in docs/tracking/README.md

Proceed with remaining [N] issues?
```

Use `AskUserQuestion` to confirm.

---

## Phase 4: Present Summary

```
CODE REVIEW SUMMARY
===================

Files Reviewed: X
Reviewers: [list of reviewer types used]
Skipped (unavailable): [list, if any]

CRITICAL ISSUES (Must Fix)
--------------------------
| Issue | Location | Reviewer | Recommended Fix |
|-------|----------|----------|-----------------|

MAJOR ISSUES (Should Fix)
-------------------------
| Issue | Location | Reviewer | Recommended Fix |
|-------|----------|----------|-----------------|

MINOR ISSUES (Nice to Fix)
--------------------------
| Issue | Location | Reviewer | Recommended Fix |
|-------|----------|----------|-----------------|

POSITIVE OBSERVATIONS
---------------------
- [Good patterns observed by reviewers]

OVERALL ASSESSMENT
------------------
- Go Idioms: X/10
- Error Handling: X/10
- Test Coverage: X/10
- Python SDK Parity: X/10

Production Ready: [Yes/No - with explanation]
```

---

## Phase 5: Offer Fixes

If issues were found, present options via `AskUserQuestion`:

1. **Fix all issues** — Apply fixes for all severities.
2. **Fix critical only** — Only address critical issues.
3. **Fix selected** — User chooses which to fix.
4. **Skip fixes** — Report only, no changes.

If user chooses to fix:

1. Apply fixes using `Edit` tool.
2. Run full quality pipeline:
   ```bash
   make check
   go test -race -cover ./...
   ```
3. Report what was fixed and verify no regressions.

---

## Phase 6: Re-review (Optional)

After fixes applied, offer via `AskUserQuestion`:

1. **Yes** — Re-run full review (loop back to Phase 1 with same team if still active).
2. **No** — Trust the fixes, proceed to cleanup.

If re-reviewing with existing team:

- Create new review tasks for fixed files only.
- Message existing reviewers (if still active) or spawn new ones.
- Include a "Prior cycle summary" in each re-review task description so the reviewer knows what was changed and why:
  ```
  PRIOR CYCLE SUMMARY (do not re-flag without new evidence):
  - <file:symbol> — changed to <short description> because <rationale>.
  ```
  Add to the reviewer prompt: "If you would flag a symbol listed in PRIOR CYCLE SUMMARY, cite new evidence (failing test, concrete bug scenario, or a correctness/safety concern not previously raised). Do not re-raise on stylistic preference alone."
- Collect and present delta results.

---

## Team Cleanup

1. Send `shutdown_request` to each reviewer, wait for confirmations.
2. `TeamDelete` to clean up team and task directories.
3. Verify directories are removed.

---

## Integration with /tdd

When called from the `/tdd` command (recommended hook point: after Phase 5 Self Code Review, before Phase 6 Validation):

- Review is **BLOCKING** — must pass before PR creation.
- Critical/major issues trigger the fix loop automatically; no user interaction for severity selection — all must be addressed.
- Findings sent back to the `/tdd` orchestrator via `SendMessage`, not presented as a summary table.
- On pass, control returns to `/tdd` Phase 6.

Wiring the actual call inside `.claude/commands/tdd.md` is a separate change; use this section as the contract for that integration.

---

## Error Handling

- **Agent timeout**: Report which reviewer timed out, continue with others' results.
- **No files to review**: Report and exit gracefully, no team created.
- **Git errors**: Report git state issue, suggest fix.
- **Fix application fails**: Report error, show manual fix instructions.
- **Team exists**: If `review-{short-id}` already exists, generate new ID and retry.
- **Plugin reviewer unavailable**: Skip with a warning in the "Skipped" line of the summary; do not abort.
