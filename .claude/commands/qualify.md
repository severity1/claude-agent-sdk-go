---
argument-hint: <issue-or-pr-number>
description: Qualify an open issue or PR against requirements and Python SDK parity before /tdd
---

# Issue and PR Qualification

Validate issue/PR #$ARGUMENTS for correctness, completeness, and Python SDK alignment. Produces a verdict report that feeds into `/tdd`.

## Phase 1: Identification & Context Gathering

Determine what #$ARGUMENTS is and find its linked counterpart:

1. **Check if it is an issue** - Run `gh issue view $ARGUMENTS --json number,title,body,state,labels,comments`
2. **Check if it is a PR** - Run `gh pr view $ARGUMENTS --json number,title,body,state,files,additions,deletions,commits,comments,headRefName`
3. **Find the linked counterpart:**
   - If PR: extract linked issue from body (`Closes #N`, `Fixes #N`, `Issue #N`, `(Issue #N)`) and fetch it
   - If issue: search for linked PRs via `gh pr list --search "issue #$ARGUMENTS"` and check open PRs referencing it
4. **Fetch all comments** on both the issue and PR (if both exist) for additional context

Display a summary:

```
Context for #$ARGUMENTS
-----------------------
Issue: #<number> - <title> (<state>)
PR:    #<number> - <title> (<state>) [or "None found"]
```

---

## Phase 2: Issue Validation

Assess whether the issue is well-defined and ready for implementation:

1. **Check issue structure** - Does the body contain:
   - [ ] Clear description of the problem or feature
   - [ ] Reproduction steps (for bugs)
   - [ ] Proposed Implementation section
   - [ ] Files to Modify section
   - [ ] Example Usage (if applicable)
2. **Read all comments** - Look for scope changes, decisions, or additional requirements added after filing
3. **Check labels** - Confirm categorization (bug, enhancement, good first issue, etc.)
4. **Extract requirements** - Build a numbered list of acceptance criteria from the issue body and comments. Include both explicit requirements and implied ones.

Output the requirements list - this feeds directly into `/tdd` Phase 2:

```
Requirements for #$ARGUMENTS
-----------------------------
1. <requirement>
2. <requirement>
...
```

**If the issue is missing critical information:** Note the gaps but continue - the verdict will reflect them.

---

## Phase 3: Python SDK Cross-Reference

Determine if this change has a Python SDK equivalent and validate alignment:

1. **Fetch official Python SDK docs:**
   ```bash
   curl -s https://platform.claude.com/docs/en/agent-sdk/python.md
   ```
2. **Search local Python SDK clone** (if `../claude-agent-sdk-python/` exists):
   - First run `git -C ../claude-agent-sdk-python pull origin main` to ensure latest changes are available
   - Check `docs/tracking/README.md` to see if this issue maps to a known Python SDK PR. If a tracker entry exists, use the Python PR number for precise cross-referencing.
   - Look for matching function/method names, types, or behavior
   - Check git log for related commits or fixes
3. **Follow referenced PRs** - If the issue body references `anthropics/claude-agent-sdk-python#NNN`, fetch that PR for context
4. **Check Python SDK issue tracker** - Search for related issues via `gh issue list -R anthropics/claude-agent-sdk-python --search "<keywords>"`

**Classify the change:**

- `PARITY_REQUIRED` - Python SDK has this feature/fix; Go must match the behavior
- `GO_SPECIFIC` - Legitimate Go-only concern (e.g., context handling, functional options, goroutine safety)
- `AHEAD_OF_PYTHON` - Feature the Python SDK does not have yet; flag for discussion

If `PARITY_REQUIRED`: document the expected behavior from the Python SDK, including API signatures, edge case handling, and any Go-specific adaptations needed. This feeds into `/tdd` Phase 3.

If Python SDK reference is unavailable (no local clone, docs fetch fails): note this in the verdict and proceed with what is available.

---

## Phase 4: Implementation Review

**If no PR exists, skip to Phase 5.**

When a PR is linked, review the implementation against the requirements:

1. **Fetch the diff** - `gh pr diff <pr-number>`
2. **Map requirements to diff** - For each requirement from Phase 2, check if the diff addresses it
3. **Check project conventions:**
   - [ ] Commits follow conventional prefixes (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`)
   - [ ] Idiomatic Go patterns (error wrapping with `%w`, context-first, functional options)
   - [ ] Tests present (table-driven, `t.Helper()` in helpers, thread-safe mocks)
   - [ ] No unnecessary exports
   - [ ] Cyclomatic complexity reasonable (threshold: 15)
   - [ ] Error handling follows project patterns
4. **Parity verification** (if `PARITY_REQUIRED`):
   - Compare Go implementation against the Python SDK behavior documented in Phase 3
   - Verify API surface, semantics, and edge cases match
   - Flag any behavioral divergence
5. **Check for common issues:**
   - Missing error paths
   - Resource leaks or missing cleanup
   - Missing context cancellation support
   - Unrelated changes bundled in

---

## Phase 5: Qualification Verdict

Produce a structured verdict. This report is designed to feed into `/tdd`:

```
Qualification Verdict: [QUALIFIED | QUALIFIED WITH ITEMS | NOT QUALIFIED]

Subject:
  Issue: #<number> - <title> (<state>)
  PR:    #<number> - <title> (<state>) [or "None"]

Requirements:
  1. <requirement> - [CLEAR | UNCLEAR | MISSING DETAIL]
  2. <requirement> - [CLEAR | UNCLEAR | MISSING DETAIL]

Python SDK Parity: [PARITY_REQUIRED | GO_SPECIFIC | AHEAD_OF_PYTHON]
  <reference details - SDK behavior, docs link, or referenced PR>

Implementation Review: [REVIEWED | NO PR | SKIPPED]
  <if reviewed, per-requirement coverage>
  1. <requirement> - [MET | PARTIALLY MET | NOT MET]
  2. <requirement> - [MET | PARTIALLY MET | NOT MET]

Action Items:
  [must-fix] <specific item>
  [should-fix] <specific item>

Recommendation:
  - QUALIFIED: "Ready for /tdd #$ARGUMENTS"
  - QUALIFIED WITH ITEMS: "Address items above, then /tdd #$ARGUMENTS"
  - NOT QUALIFIED: "<specific reason and suggested next step>"
```

**Verdict criteria:**

- `QUALIFIED` - Issue is valid, well-scoped, requirements are clear, parity is understood. Ready for `/tdd`.
- `QUALIFIED WITH ITEMS` - Mostly ready but has specific items to address first. List each with `must-fix` or `should-fix`.
- `NOT QUALIFIED` - Significant gaps: unclear requirements, invalid premise, parity conflict, or the issue needs rethinking.

---

## Error Recovery

- **Invalid number:** Report that #$ARGUMENTS was not found as an issue or PR
- **`gh` not authenticated:** Report and suggest `gh auth login`
- **Python SDK unavailable:** Proceed with qualification but note parity could not be fully verified
- **No linked issue or PR:** Proceed with what is available; note the gap in the verdict
