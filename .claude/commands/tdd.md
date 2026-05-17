---
argument-hint: <issue-number>
description: Autonomous TDD development cycle for GitHub issues with quality gates
---

# TDD Development Cycle

Execute complete TDD workflow for GitHub issue #$ARGUMENTS with built-in quality gates and Python SDK parity checks.

## Phase 1: Pre-flight Checks

Verify the development environment is ready:

1. **Check working directory status** - Ensure no uncommitted changes exist
2. **Verify current branch** - Should be on `main` branch
3. **Pull latest changes** - Run `git pull` to sync with remote
4. **Check for existing PRs** - Search for open PRs linked to issue #$ARGUMENTS to avoid duplicate work
5. **Verify Go environment** - Run `go version` and ensure toolchain is available
6. **Sync Python SDK** - Run `git -C ../claude-agent-sdk-python pull origin main` to ensure local Python SDK reference is current
7. **Check for blocking dependencies** - Read issue body and look for "Depends on" or "Blocked by" sections

**STOP and report to user if any check fails** - Don't proceed until issues are resolved.

---

## Phase 2: Issue Validation

Retrieve and analyze issue #$ARGUMENTS:

1. **Fetch issue details** - Get title, body, labels, milestone, state via `gh issue view`
2. **Read all comments** - Check for additional context or decisions made
3. **Validate issue completeness** - Check if issue contains:
   - Summary or description of the feature/fix
   - Proposed Implementation section (most issues have this)
   - Files to Modify section
   - Example Usage (if applicable)

**If incomplete:** Report gaps to user and ask if should proceed anyway.
**If complete:** Display issue summary and continue.

---

## Phase 3: Discovery & Planning

**Enter plan mode for this entire phase.** Call `EnterPlanMode` at the start of Phase 3 so all discovery, design decisions, and parity tradeoffs are captured as a plan the user approves before any code is written. The phase ends with `ExitPlanMode` at the *Critical Checkpoint* below. If already in plan mode, `EnterPlanMode` is a no-op — proceed.

### Design Principle: Two Co-Equal Parity Gates

This work passes through **two co-equal mandatory gates**. Both must pass. Neither compensates for the other:

1. **Observable-behavior parity with Python SDK** — JSON wire format, public API surface, CLI flags, message shapes, and semantics exposed to consumers. This is the *contract* the SDK delivers.
2. **Idiomatic Go delivery** — the *shape* of the code that delivers that contract: nil-safety, context-first, `fmt.Errorf` with `%w` wrapping, zero-value usability, small focused interfaces, gofmt/golangci-lint conformance, gocyclo under 15, channel-based concurrency, no unnecessary exports. The Go shape is itself a parity gate, not a polish step applied after the fact.

Parity is the *what*. Idiomatic Go is the *how*. A faithful translation of Python internals that violates Go idiom is a Gate-2 failure even if the wire format is exact. Conversely, a beautifully idiomatic Go API that drifts from the wire contract is a Gate-1 failure.

**Internal mechanics** (private struct layout, helper return types, control flow shape, dispatch strategy) are **not** parity targets. When a Go idiom and a Python internal shape conflict, choose the Go idiom and record the deliberate divergence in the plan (and later in the commit message + PR description — never in source comments). The plan presented at `ExitPlanMode` must name each divergence explicitly so the user approves it up front rather than discovering it in review.

### Codebase Exploration

Understand existing patterns to match the project's conventions:

1. **Review Python SDK Reference:**
   - **Check parity tracker first** - Read `docs/tracking/README.md` to find if this issue maps to a tracked Python SDK PR. If a tracker entry exists, use the Python PR number as the authoritative reference.
   - **Read Python source directly** - Tracker notes are intentionally brief summaries, not the full spec. Read the actual Python source files at `../claude-agent-sdk-python/` for exact behavior:
     - `src/claude_agent_sdk/types.py` - all public type definitions (messages, options, hooks)
     - `src/claude_agent_sdk/client.py` - public Client interface
     - `src/claude_agent_sdk/_internal/client.py` - internal client/transport implementation
     - `src/claude_agent_sdk/_internal/transport/subprocess_cli.py` - subprocess/protocol logic
     - `src/claude_agent_sdk/_internal/message_parser.py` - message parsing
     - `src/claude_agent_sdk/_internal/sessions.py` - session management
     - `src/claude_agent_sdk/_internal/session_mutations.py` - session mutations (rename, tag, etc.)
   - **Verify exact details from Python source:** field names, JSON tags, optional vs required, default values, serialization behavior - flag any divergence that isn't justified by Go idiom
   - **Fetch official documentation** using `curl -s https://platform.claude.com/docs/en/agent-sdk/python.md` for public API signatures if helpful
   - Note any Go-specific adaptations needed (e.g., pointer for optional fields, interface for union types)

2. **Discover Existing Patterns:**
   - Search for similar implementations in the codebase
   - Review `client_test.go` as the gold standard for testing patterns
   - Check existing type definitions and interfaces
   - Understand error handling patterns (`fmt.Errorf` with `%w`)

3. **Identify Files to Modify:**
   - Map issue's "Files to Modify" section to actual paths
   - Check for related test files that need updates
   - Note any re-exports needed in `types.go`

4. **Review Related Context:**
   - Read issues mentioned in "Depends on" or "Blocks" sections
   - Check closed PRs for similar features

### Create TDD Implementation Plan

Based on exploration and issue requirements, create detailed plan with:

1. **RED Phase - Tests First:**
   - Test cases to write (based on Python SDK behavior)
   - Expected failures before implementation
   - Table-driven test structure if multiple cases

2. **GREEN Phase - Implementation:**
   - Minimal code to make tests pass
   - Files to create/modify
   - Types and interfaces needed

3. **BLUE Phase - Refactoring:**
   - Code quality improvements
   - Pattern alignment
   - Documentation updates

4. **Acceptance Criteria Mapping:**
   - Map each requirement to test cases
   - Identify how each will be verified

5. **Size & Complexity Assessment:**
   Score the issue against these signals and record the result in the plan:
   - Files touched (create + modify)
   - Independent types/subsystems introduced (e.g. unrelated message types, separate protocol surfaces)
   - New public API surface (exported symbols, re-exports in `types.go`)
   - Python PRs covered by this issue (sometimes one Go issue replays several)

   Decide on the execution strategy based on the score:
   - **Small (default):** ≤3 files, one subsystem, single Python PR — execute sequentially in a single agent context. Do **not** spawn sub-agents.
   - **Medium:** 4-8 files or 2-3 independent types — stay sequential for RED/GREEN/BLUE, but in Phase 5 delegate the self code review to parallel `code-reviewer` agents scoped per file or per subsystem (inline `Agent` spawns, never `TeamCreate`).
   - **Large:** 9+ files, multiple subsystems, or multiple Python PRs — first ask the user whether to split into sub-issues. If the user confirms keeping it as one cycle, then: still sequential RED/GREEN/BLUE (TDD ordering must hold), but spawn parallel reviewers in Phase 5 *and* use an `Explore` agent during Phase 3 to map existing patterns per subsystem instead of doing it inline.

   Never parallelize RED or GREEN test/impl writing across agents — pattern drift and merge conflicts in a single branch outweigh the speedup. Parallelism is a *review* tool, not a *writing* tool.

### Critical Checkpoint: User Approval

**Call `ExitPlanMode` to present the plan and await user approval before proceeding.** The plan must include: RED/GREEN/BLUE breakdown, size/complexity score and chosen execution strategy, and an explicit list of any deliberate Go-idiom divergences from Python internals.

Do NOT continue to Phase 4 until user approves the plan.

---

## Phase 4: TDD Implementation

### Create Feature Branch

Generate branch name from issue (e.g., Issue #34 "Add plugins support" becomes `feature/issue-34-add-plugins-support`) and create the branch.

### RED Phase: Write Failing Tests

1. **Write test cases first** based on Python SDK behavior
2. **Run tests to verify they fail:**
   ```bash
   go test ./... -v
   ```
3. **Commit failing tests:**
   ```
   test: add tests for <feature> (Issue #$ARGUMENTS)

   - Test case 1 description
   - Test case 2 description
   - Tests expected to fail until implementation
   ```

### GREEN Phase: Implement to Pass Tests

1. **Write minimal implementation** to make tests pass
2. **Run tests to verify they pass:**
   ```bash
   go test ./... -v
   ```
3. **Commit implementation:**
   ```
   feat: implement <feature> (Issue #$ARGUMENTS)

   - Implementation detail 1
   - Implementation detail 2
   - All tests now passing
   ```

### BLUE Phase: Refactor (if needed)

1. **Run quality checks:**
   ```bash
   go fmt ./...
   go vet ./...
   golangci-lint run
   gocyclo -over 15 .
   deadcode -test=true \
     -filter='github.com/severity1/claude-agent-sdk-go/internal/...' \
     ./examples/... ./internal/...
   # Or one-shot: make check
   ```
2. **Fix any issues found**
3. **Sweep for comment anti-patterns** (these almost always sneak into a fresh implementation; strip them now rather than letting a follow-up audit do it later):
   ```bash
   # Cross-SDK parity claims and "following X patterns" XREFs
   rg -n '^\s*//.*(Matches Python SDK|Python SDK.*exactly|Added in Python SDK PR|following.*patterns)' --type go
   # ASCII banner separators
   rg -n '^\s*//\s*={5,}|^\s*//\s*-{5,}' --type go
   # PR/Issue trailers in comments
   rg -n '^\s*//.*\b(?:PR|Issue) #\d+' --type go
   # T### task-tracker prefixes
   rg -n '^\s*//\s*T\d+:' --type go
   # NOLINT-WHY: nolint with inline trailing explanation
   rg -n '//nolint:[A-Za-z,]+\s+//' --type go
   # NOLINT-WHY: standalone comment immediately preceding a nolint that paraphrases it
   rg -nB1 '//nolint:' --type go | rg -i 'exceeds|complexity|threshold|to satisfy|because|allow|suppress'
   # RATIONALE: decision-history prose in source
   rg -n '^\s*//.*\b(we chose|we decided|we went with|chose .* over|preferred .* over|decided to use)' --type go
   # EXTRACTION-HIST: past-tense extraction/refactor narration
   rg -n '^\s*//.*\b(Extracted from|Pulled out of|Moved (out )?of|Refactored to|Was (originally|previously) inlined|Used to be|Lifted from)' --type go
   # EXTRACTION-HIST: "to keep/reduce X under <linter>" rationale that pairs with extraction
   rg -n '^\s*//.*\bto (keep|reduce|stay under|satisfy)\b.*\b(complexity|gocyclo|cyclomatic|budget|funlen|lint)\b' --type go
   ```
   Each match should be evaluated and either removed or reworded — see the **Comment & Docstring Checklist** in Phase 5 for the rationale. The implementation is the contract; parity context lives in `docs/tracking/`, decisioning history and rationale in git log + PR descriptions, and linter directives (`//nolint:*`, `//go:noinline`) are self-documenting — never apologize for them in a sibling comment. If the suppression is non-obvious enough to warrant explanation, the right fix is to eliminate the suppression (extract a helper, restructure the code), not to add a comment that paraphrases the rule name.
4. **Commit refactoring (if changes made):**
   ```
   refactor: improve <feature> (Issue #$ARGUMENTS)

   - Quality improvement 1
   - Code cleanup
   ```

### Push Feature Branch

Push the feature branch to remote with upstream tracking.

---

## Phase 5: Self Code Review

**Execution strategy follows the size score from Phase 3:**
- **Small:** inline self-review using the checklists below.
- **Medium / Large:** spawn parallel `code-reviewer` agents via the `Agent` tool (one per file or subsystem). Use inline `Agent` spawns only — never `TeamCreate`, `TeamDelete`, or broadcast. Each spawned reviewer receives the checklists below as its brief plus the list of deliberate Go-idiom divergences from the approved plan (so it does not re-flag them). Collect findings, dedupe, then fix.

Before finalizing, review ALL implemented code for:

### Go Standards Checklist (Gate 2 — Idiomatic Go is a co-equal parity gate, not a polish step):

If observable behavior matches Python but the code shape is un-Go (Python translated into Go syntax), the implementation does NOT pass parity. Fix the shape; do not defer to a follow-up commit.

- [ ] Idiomatic Go patterns followed throughout
- [ ] Proper error handling with `fmt.Errorf` and `%w` wrapping
- [ ] Context-first for blocking operations (`context.Context` as first param)
- [ ] Nil-safety: every pointer dereference is guarded or invariant-proven
- [ ] No unnecessary exports (lowercase unexported unless needed by external consumers)
- [ ] Interfaces are small (1-3 methods) and focused; named for behavior, not for role
- [ ] Proper use of defer for cleanup; bounded goroutine lifetimes; no leaks
- [ ] Zero-value usability where reasonable; constructors only when invariants demand them
- [ ] `gofmt -s` clean, golangci-lint clean, gocyclo under 15 — extracted helpers, not `//nolint:gocyclo`
- [ ] Go idioms chosen over Python internal shape where they conflict; each deliberate divergence matches the list in the approved Phase 3 plan and is noted in the commit message + PR description (not in source comments)

### Security Checklist:
- [ ] No hardcoded secrets or API keys
- [ ] Input validation at system boundaries
- [ ] Buffer limits enforced (1MB protection)
- [ ] No command injection vulnerabilities

### Testing Checklist:
- [ ] Table-driven tests for multiple cases
- [ ] Test helpers call `t.Helper()`
- [ ] Thread-safe mocks with proper mutex usage
- [ ] 100% behavioral parity with Python SDK
- [ ] Edge cases covered (nil, empty, error conditions)
- [ ] No placeholder or dummy test code

### Performance Checklist:
- [ ] No goroutine leaks (proper cleanup)
- [ ] Proper resource cleanup in all paths
- [ ] Efficient buffer management
- [ ] Context cancellation respected

### Comment & Docstring Checklist:

Source comments describe **current Go behavior only**. Parity context, decisioning history, design rationale, refactor narration, and PR/issue refs are *derived artifacts* — they live in `docs/tracking/`, git commit messages, and the PR description, not inline. Comments that duplicate any of those rot independently and mislead future readers. Linter directives (`//nolint:*`, `//go:noinline`, `//go:build`) are self-documenting — apologizing for them in a sibling comment is redundant noise; if the suppression is non-obvious enough to need explanation, eliminate the suppression itself.

- [ ] No "Matches Python SDK X exactly" or similar cross-SDK comparison claims on types, constants, or methods
- [ ] No "Added in Python SDK PR #N" / "(PR #N fix)" / "Issue #N" trailers on comments
- [ ] No ASCII banner separators (`// =====...`, `// -----...`) — use plain section comments or no separator
- [ ] No `T###:` task-tracker prefixes on test functions or block comments
- [ ] No "following X patterns" XREFs (e.g. "following client_test.go patterns") — established patterns live in CLAUDE.md, not as inline justifications
- [ ] No inter-SDK comparison in the `doc.go` package overview — describe what the Go package does, not how it relates to Python
- [ ] No tutorial-style prose in production code (acceptable only in `examples/` and `doc.go` package overview)
- [ ] No decision-history prose ("we chose X over Y", "preferred map over slice because", "decided to use mutex instead of channel") — the current shape of the code IS the decision; rationale belongs in the commit message and PR description
- [ ] No past-tense refactor / extraction narration ("Extracted from foo() to keep bar under gocyclo", "Pulled out of the switch", "Was originally inlined in Connect()") — describe the helper's current role in present tense; if extraction is a load-bearing invariant, name the invariant ("Kept standalone so the dispatch switch stays under gocyclo budget")
- [ ] No comments explaining `//nolint:*` directives — the directive name already names the rule; the suppression is the explanation. If the suppression itself is non-obvious, restructure the code (extract a helper, split the function) so the suppression goes away
- [ ] No commented-out code; no author/date stamps; no emojis or em-dashes
- [ ] Default to no comment. Add one only when the *why* is non-obvious: a hidden constraint, subtle invariant, surprising behavior, or wire-format detail that's not visible from the code alone
- [ ] Doc comments on exported identifiers start with the identifier name (godoc convention: `// FooBar does X`, not `// Does X`)
- [ ] Doc comments on exported identifiers are concise — one sentence ideally, no multi-paragraph essays (only the `doc.go` package overview is exempt)

When parity context or design rationale for a field or type is genuinely useful to a maintainer, put it in:
- The commit message — full PR reference + rationale lives here permanently
- `docs/tracking/README.md` — the parity tracker row owns the Python-PR-to-Go-PR mapping
- The PR body — "this implements Python PR #N, …" and "we picked approach X because …" go here for reviewers

**If issues found:** Fix them and create an additional commit with description of what was fixed.

---

## Phase 6: Validation

### Run Full Test Suite

```bash
go test -cover -race ./...
```

Verify:
1. **All tests pass** - No failures allowed
2. **Coverage acceptable** - Check coverage report
3. **No race conditions** - Race detector finds nothing

### Two-Gate Alignment Check

Verify **both** gates from Phase 3 explicitly. Passing one does not compensate for failing the other.

**Gate 1 — Observable-behavior parity with Python SDK:**

1. **Re-read the Python source** at `../claude-agent-sdk-python/` for each implemented feature - verify:
   - Type names and JSON tags match exactly (grep the Python source for every field name; do not trust tracker prose)
   - Optional vs required fields match (Go: pointer for optional, value for required)
   - Behavior for edge cases (nil/None, empty collections, error paths) matches
   - Any field the Python SDK omits with `omitempty` (or equivalent) is omitted in Go too
   - CLI flag names and constant string values are byte-identical to Python source
2. **Reference official docs** - `curl -s https://platform.claude.com/docs/en/agent-sdk/python.md` for public API signatures
3. **Verify 100% parity** on observable behavior for all implemented features

**Gate 2 — Idiomatic Go delivery:**

1. **Re-walk the diff** asking: would a senior Go reviewer write this, or does it read like Python translated into Go syntax?
2. **Verify each item in the Go Standards Checklist from Phase 5** — context-first, error wrapping, nil-safety, small interfaces, gofmt -s clean, gocyclo under 15 via extracted helpers (not `//nolint`), no unnecessary exports
3. **Confirm CLAUDE.md `## Detected Patterns` and per-module CLAUDE.md notes are followed** — these encode the repo's idiomatic Go judgment calls

**Both gates passed:**

4. **Document any intentional deviations** from Python internals (Go-specific adaptations, e.g., sealed interface for union types, functional options for keyword-only args) — note them in the commit message and PR description, not in source comments
5. **Update parity tracker** - If this issue corresponds to a tracked Python PR in `docs/tracking/README.md`, update the entry: set Go Status to `done` and fill in the Go PR number

### Test Authenticity Verification

1. **No placeholder code** - All tests are real and meaningful
2. **No dummy implementations** - Production-ready code only
3. **Proper assertions** - Tests actually verify behavior

### Run Benchmarks (if applicable)

```bash
go test -bench=. -benchmem ./...
```

**STOP if validation fails:** Report issues to user and await instructions.

---

## Phase 7: PR Creation & Merge

### Create Pull Request

Create PR with:
- Title: `feat: <Issue Title> (Issue #$ARGUMENTS)`
- Body containing:
  ```markdown
  ## Summary
  <1-2 sentence overview>

  ## Changes

  ### Files Created
  - `path/to/file.go` - Description

  ### Files Modified
  - `path/to/file.go` - What changed

  ## Test Plan
  - [ ] All tests passing
  - [ ] Coverage maintained/improved
  - [ ] Race detector clean
  - [ ] Python SDK parity verified

  ## TDD Cycle
  - RED: Tests written first (commit SHA)
  - GREEN: Implementation added (commit SHA)
  - BLUE: Refactored (commit SHA, if applicable)

  Closes #$ARGUMENTS
  ```

### Interactive Checkpoint: PR Review

Display PR URL to user and ask them to review.

Options:
1. **Approve** - Proceed to merge
2. **Request changes** - Wait for user edits
3. **Reject** - Close PR and rollback

### After User Approval: Merge PR

1. Merge with squash and delete branch
2. Checkout main
3. Pull latest changes

### Document Issue Completion

Add comprehensive completion comment to issue #$ARGUMENTS with:
- Implementation Summary
- Files Created/Modified
- Test Coverage results
- Python SDK parity confirmation
- Link to merged PR

### Verify Issue Auto-Closed

Check that issue #$ARGUMENTS state is now "CLOSED".

---

## Completion Summary

Display final summary to user:

```
TDD Development Cycle Complete for Issue #$ARGUMENTS

Phase 1: Pre-flight Checks - Done
Phase 2: Issue Validation - Done
Phase 3: Planning (User Approved) - Done
Phase 4: TDD Implementation - Done
  - RED: Tests written
  - GREEN: Implementation complete
  - BLUE: Refactored (if applicable)
Phase 5: Code Review - Done
Phase 6: Validation - Done
Phase 7: PR Merged - Done

PR: #<number> (merged and branch deleted)
Issue: #$ARGUMENTS (closed)
Branch: main (updated)

Test Results:
- Tests: X passed
- Coverage: XX%
- Race conditions: None
```

---

## Error Recovery

If any phase fails:

1. **Pre-flight/Validation Failures:** Report to user, provide fix suggestions, stop execution
2. **Test Failures (RED phase):** Expected - this is TDD. Continue to GREEN phase
3. **Test Failures (GREEN phase):** Fix implementation until tests pass
4. **Lint/Vet Failures:** Auto-fix where possible with `go fmt`, report unfixable errors
5. **PR Creation Failures:** Report error, provide manual PR creation command

**Branch is preserved** - user can manually inspect, fix, and continue.
