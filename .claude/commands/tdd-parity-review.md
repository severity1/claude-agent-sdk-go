---
description: Parity-focused review using grumpy-gopher to verify a branch faithfully implements a Python SDK PR with idiomatic Go
allowed-tools: Read, Grep, Glob, Bash, Agent, WebFetch
---

# TDD Parity Review

> **REVIEW ONLY.** This command reports findings. It never makes changes. After presenting results, stop and let the user decide what to fix.

Spawns a `grumpy-gopher` agent to review the current branch against one or more Python SDK parity items. Verifies faithful implementation, correct wire format, no regressions, idiomatic Go, and example coverage - in that priority order.

## Usage

```
/tdd-parity-review [python-pr-numbers]
```

Examples:
- `/tdd-parity-review` — reviews all pending items on the current branch (auto-detected from tracker)
- `/tdd-parity-review 506` — reviews Python PR #506 specifically
- `/tdd-parity-review 506 516` — reviews both PRs

## Step 1: Gather Context

Run these in parallel:

```bash
git diff main...HEAD --stat
git log main..HEAD --oneline
git diff main...HEAD -- <substantive Go files, exclude CLAUDE.md/auto-memory>
```

Read `docs/tracking/README.md` to get the spec for each target PR: expected types, field names, wire format, JSON tags, method signatures.

If `$ARGUMENTS` is empty, infer target PRs from the tracker: find items whose Go Status is `pending` and whose description matches files changed on this branch.

## Step 2: Locate Python SDK Source

The Python SDK lives at `../claude-agent-sdk-python/src/claude_agent_sdk/`. Key files:
- `_internal/message_parser.py` - message parsing and wire format
- `types.py` - type definitions, field names, literals
- `_internal/query.py` - control protocol subtypes and request shapes
- `_internal/client.py` - client API surface

Read the relevant Python source for each target PR to establish ground truth. Do not assume field names or constant values - grep the Python source to confirm every single one.

## Step 3: Build and Run Examples

Before spawning the reviewer, check the examples directory:

```bash
# Verify all examples still compile after branch changes
go build ./examples/...

# List examples and their entry points
ls examples/
```

For each example that compiles, attempt to run it with a short timeout to catch runtime panics or obvious breakage:

```bash
# Run each example binary briefly - they may need ANTHROPIC_API_KEY
# Note which ones succeed, which fail, and why (missing env, panic, etc.)
```

Document:
- Which examples compile cleanly
- Which examples fail to compile and why
- Which examples exercise functionality added by the target PRs
- Which examples are missing coverage for new functionality

## Step 4: Spawn grumpy-gopher

Spawn a single `grumpy-gopher` agent with a self-contained prompt that includes:

1. **What this branch claims to implement** - list each Python PR number, its title, and the tracker spec

2. **Python SDK ground truth** - paste the relevant Python source snippets. Include:
   - Every struct field name and its JSON key (grep `types.py` for each)
   - Every constant string value (grep `_internal/query.py` for subtypes, `types.py` for Literals)
   - Every CLI flag name (grep `_internal/cli.py` or equivalent)
   - Every enum/Literal value

3. **Files to review** - the substantive changed files from Step 1

4. **Review mandate** (in priority order):

   - **100% wire format audit** - for every struct with JSON tags, verify each field name against Python source. For every constant, verify the actual string value (not just the Go name) against Python. For every control protocol subtype, verify against `_internal/query.py`. Do not assume - grep and confirm. Flag any field name, constant value, or subtype that cannot be confirmed from Python source.

   - **Python SDK parity** - does the Go shape match Python's observable behavior? Public API surface (method names, parameter types, return types), semantics (what the method does, what errors it returns, what fields are populated). Internal mechanics may differ - prefer idiomatic Go over mirroring Python internals.

   - **No regressions** - do existing tests still pass? Are new tests correct and comprehensive? Do tests use the actual wire format (not a shape that matches a bug)?

   - **Idiomatic Go + repo conventions** - context-first, `fmt.Errorf` with `%w`, no unnecessary exports, interfaces small and focused, cyclomatic complexity under 15. Check established patterns in CLAUDE.md.

   - **Code quality** - KISS/YAGNI/DRY, no dead code, no over-engineering.

   - **Examples coverage** - for each new public API added by the target PRs: is there an example demonstrating it? Do existing examples still compile and run correctly? Are any examples stale or misleading after the changes? List specific examples that should be added or updated to enable functional live testing.

5. **Verification steps** to run: `go test ./...`, `go vet ./...`, `go build ./examples/...`

6. **Output format**:
   ```
   BLOCKER: [wrong behavior, missing parity, regression, tests that mask a bug]
   WARNING: [behavioral change, subtle issue, flaky test]
   MINOR:   [style, minor divergence, additive difference]
   GOOD:    [correct patterns, solid test coverage, faithful parity]

   EXAMPLES:
     MISSING: [new API with no example - describe what example should show]
     STALE:   [existing example that needs updating - describe what changed]
     OK:      [examples that cover this functionality correctly]

   VERDICT: [ship / do not ship + one sentence why]
   ```

   Each finding must include file:line. For each blocker: what the Python SDK does, what the Go code does, and the exact fix needed (describe the fix - do not apply it).

## Step 5: Present Results

Relay the grumpy-gopher findings directly. Do not summarize or soften. If there are blockers, state them first and clearly.

After presenting findings, note:
- Which tracker items are fully implemented and ready
- Which tracker items are missing or incomplete
- Which examples need to be added or updated for functional live testing
- Whether the branch is ready to PR or needs more work

**Stop here. Do not apply any fixes.** The user will decide what to address based on the findings.

## Parity Standard

The goal is parity with the **observable behavior** of the Python SDK:
- Wire format: JSON field names, constant values, message shapes, CLI flags
- Public API surface: method names, parameter types, return types
- Semantics: what the method does, what errors it returns, what fields are populated

Parity on **internal mechanics** is not a goal. Since this is Go, prefer:
- Idiomatic Go over mirroring Python internals
- Established repo patterns (see CLAUDE.md `## Detected Patterns`) over Python-shaped code
- A superior Go pattern over an existing repo pattern only when the improvement is clear and non-disruptive

When a Go idiom and a Python internal shape conflict, choose the Go idiom and note the deliberate divergence.
