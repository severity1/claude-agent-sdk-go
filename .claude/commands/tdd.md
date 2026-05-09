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

### Design Principle: Go Idioms Are First-Class

Parity target is **observable behavior**: JSON wire format, public API surface, CLI flags, message shapes, and semantics exposed to consumers. Internal mechanics (private struct layout, helper return types, control flow shape) are **not** parity targets.

When a Go idiom and a Python internal shape conflict, choose the Go idiom — nil-safety, context-first, `fmt.Errorf` with `%w` wrapping, zero-value usability, small focused interfaces, gofmt/linter conformance, channel-based concurrency — and record the deliberate divergence in the plan (and later in the commit message). The plan presented at `ExitPlanMode` must name each divergence explicitly so the user approves it up front rather than discovering it in review.

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
3. **Commit refactoring (if changes made):**
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

### Go Standards Checklist:
- [ ] Idiomatic Go patterns followed
- [ ] Proper error handling with `fmt.Errorf` and `%w` wrapping
- [ ] Context-first for blocking operations (`context.Context` as first param)
- [ ] No unnecessary exports (lowercase unexported unless needed)
- [ ] Interfaces are small and focused
- [ ] Proper use of defer for cleanup
- [ ] Go idioms chosen over Python internal shape where they conflict; each deliberate divergence matches the list in the approved Phase 3 plan and is noted in the commit message

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

### Python SDK Alignment Check

1. **Re-read the Python source** at `../claude-agent-sdk-python/` for each implemented feature - verify:
   - Type names and JSON tags match exactly
   - Optional vs required fields match (Go: pointer for optional, value for required)
   - Behavior for edge cases (nil/None, empty collections, error paths) matches
   - Any field the Python SDK omits with `omitempty` should be omitted in Go too
2. **Reference official docs** - `curl -s https://platform.claude.com/docs/en/agent-sdk/python.md` for public API signatures
3. **Verify 100% parity** on all implemented features
4. **Document any intentional deviations** (Go-specific adaptations, e.g., sealed interface for union types)
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
