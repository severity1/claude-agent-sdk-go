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

   - **Docs and tracking reconciliation** - implementation often deviates from tracker prose, module CLAUDE.md notes, in-code comments, or auto-memory entries — sometimes because the original spec was imprecise, sometimes because the implementation chose a superior approach (better naming, idiomatic Go pattern, cleaner architecture, broader coverage than Python). When implementation is the source of truth (it was reviewed and merged) but documentation describes something different, **the docs are wrong, not the code**. Compare every implementation detail against:
     - `docs/tracking/README.md` row for each target PR (subtype strings, type names, file paths, scope notes)
     - `CLAUDE.md` at the project root (`## Detected Patterns`, `## Code Conventions`, parity notes)
     - Per-module `CLAUDE.md` files (`internal/*/CLAUDE.md`, `examples/CLAUDE.md`) for module-specific conventions and patterns
     - In-code comments and docstrings on changed types/functions
     - Auto-memory entries in `~/.claude/projects/<repo>/memory/` (frontmatter `description`, MEMORY.md hooks)

     Flag every divergence and classify it:
     - **Imprecise**: doc text describes something close-but-wrong (e.g., subtype prose says `"get_mcp_status"` but the wire string is actually `"mcp_status"`).
     - **Stale**: doc text describes the pre-implementation state when the implementation has moved on (e.g., comment says "Go SDK extensions" for constants that are in fact Python parity).
     - **Underspecified**: implementation added structure/coverage/safety the docs never described (e.g., a constructor pattern, a defensive nil-guard, a richer error type) — flag so docs can be expanded.
     - **Superior**: implementation chose a deliberately better-than-Python approach (idiomatic Go pattern, simpler architecture, broader API surface) — flag so docs explicitly note the deliberate divergence and the rationale, instead of leaving it unstated.

     Report these under `DOCS RECONCILIATION` in the output (separate section from BLOCKER/WARNING/MINOR). Each entry must name the file:line of the doc to update AND the file:line of the implementation that proves the doc is wrong. Do not apply the docs fix - only flag it.

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

   DOCS RECONCILIATION:
     IMPRECISE:      [doc-file:line says X, implementation-file:line shows Y - fix doc to match]
     STALE:          [doc describes pre-implementation state, implementation has moved on]
     UNDERSPECIFIED: [implementation added structure/coverage docs never described - expand doc]
     SUPERIOR:       [implementation deliberately better than Python/spec - doc should note rationale]

   VERDICT: [ship / do not ship + one sentence why]
   ```

   Each finding must include file:line. For each blocker: what the Python SDK does, what the Go code does, and the exact fix needed (describe the fix - do not apply it). For each `DOCS RECONCILIATION` entry: name both the doc file:line that is wrong AND the implementation file:line that proves it.

## Step 5: Present Results

Relay the grumpy-gopher findings directly. Do not summarize or soften. If there are blockers, state them first and clearly.

After presenting findings, note:
- Which tracker items are fully implemented and ready
- Which tracker items are missing or incomplete
- Which examples need to be added or updated for functional live testing
- **Which docs/tracking entries need reconciliation** (tracker rows, CLAUDE.md notes, in-code comments, auto-memory) — distinguish "doc is just imprecise" from "implementation chose a superior approach and docs should record that as a deliberate divergence"
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

## Docs as a derived artifact

Implementation is the source of truth on a merged or review-ready branch — tracker prose, CLAUDE.md notes, in-code comments, and auto-memory entries are derived artifacts that must follow the implementation, not the other way around. When the reviewer finds doc/code mismatch:

- **The implementation is right by default.** It was reviewed; the docs may have been written from memory, copied from an earlier draft, or left untouched when the implementation evolved.
- **Superior-implementation drift is real.** When the implementation deliberately diverges from the tracker spec or Python SDK to be better-than-Python (clearer naming, idiomatic Go, broader API surface, stronger invariants), the docs MUST record this as an intentional divergence with a one-line rationale — silent superiority becomes invisible to future contributors and they may "fix" it back.
- **Imprecise tracker prose is a common pitfall.** Tracker rows often describe Python behavior in summary form (e.g., "subtype `get_mcp_status`") that doesn't survive grep against the actual Python wire string. The reviewer must check the wire string, not the tracker prose.
- **Auto-memory and CLAUDE.md drift compounds.** A wrong note in `internal/<module>/CLAUDE.md` gets re-rendered into project root CLAUDE.md and into auto-memory entries. Fix at the root cause (the module CLAUDE.md or the source comment), not at every downstream copy.

The reviewer reports these under `DOCS RECONCILIATION` so the user can fix docs before opening the PR (or as a follow-up cleanup commit), without conflating docs polish with code blockers.
