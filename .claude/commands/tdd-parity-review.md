---
description: Parity-focused review using grumpy-gopher to verify a branch faithfully implements a Python SDK PR with idiomatic Go
allowed-tools: Read, Grep, Glob, Bash, Agent, WebFetch
---

# TDD Parity Review

> **REVIEW ONLY.** This command reports findings. It never makes changes. After presenting results, stop and let the user decide what to fix.

Spawns a `grumpy-gopher` agent to review the current branch against one or more Python SDK parity items.

**Two co-equal parity gates** are checked. Both are mandatory. A violation of either is a blocker:

1. **Observable-behavior parity with Python SDK** — wire format (JSON field names, constants, message shapes, CLI flags), public API surface (method/type/parameter names), and semantics (what each call does, what errors return, what fields populate).
2. **Idiomatic Go delivery** — the *shape* of the code that delivers the parity contract: context-first, `fmt.Errorf` with `%w` wrapping, nil-safety, small focused interfaces, gofmt/golangci-lint clean, gocyclo under 15, channel-based concurrency, zero-value usability, no unnecessary exports.

Parity is the *what*. Idiomatic Go is the *how*. Neither is negotiable; neither is a polish step. Mirroring Python *internals* is not a goal — when a Go idiom and a Python internal shape conflict, choose the Go idiom and record the deliberate divergence. Beyond the two gates, the reviewer also checks regressions, example coverage, comment hygiene, and docs reconciliation.

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

4. **Review mandate** — two co-equal mandatory gates, followed by cross-cutting concerns. The two gates are NOT ordered; failing either is a blocker. The reviewer reports findings under both gates with equal severity weight.

   ### Gate 1 — Observable-behavior parity with Python SDK (mandatory)

   - **100% wire format audit** - for every struct with JSON tags, verify each field name against Python source. For every constant, verify the actual string value (not just the Go name) against Python. For every control protocol subtype, verify against `_internal/query.py`. Do not assume - grep and confirm. Flag any field name, constant value, or subtype that cannot be confirmed from Python source.

   - **Public API surface** - does the Go shape match Python's observable behavior? Method/type/parameter/return names, optional vs required field discrimination, error types returned, fields populated on success vs failure. Internal mechanics may differ; observable surface may not.

   - **Semantics** - same inputs produce equivalent outputs; same edge cases (nil/None, empty, malformed) produce equivalent behavior; same env-var/config knobs have equivalent effects.

   ### Gate 2 — Idiomatic Go delivery (mandatory)

   The shape of the Go code is itself a parity gate. Observable behavior matching Python is necessary but not sufficient — the code that delivers that contract must look like Go a senior Go reviewer would write, not like Python translated into Go syntax. Flag at the same severity as wire-format violations:

   - **Context-first** — every blocking exported function accepts `context.Context` as the first parameter
   - **Error wrapping** — `fmt.Errorf("...: %w", err)` for chains; sentinel errors only when callers need `errors.Is`
   - **Nil-safety** — every pointer dereference is guarded or invariant-proven; never assume CLI/peer populated a field
   - **Interface design** — interfaces are small (1-3 methods), focused, named for behavior; no `XxxManager` / `XxxService` god-interfaces
   - **No unnecessary exports** — identifiers stay unexported unless an external consumer needs them; getters only when invariants require them
   - **gofmt / golangci-lint clean** — CI runs `gofmt -s`; verify with `gofmt -s -l .`
   - **gocyclo under 15** — extract helpers (e.g. `buildToolsListResult` from `routeMcpMethod`) rather than reaching for `//nolint:gocyclo`
   - **Channel-based concurrency** — goroutine lifetimes bounded by context, no leaks, no busy-wait
   - **Zero-value usability** — types are usable without explicit construction where reasonable; constructors only when invariants demand them
   - **Repo conventions** — every pattern in CLAUDE.md (`## Detected Patterns`, `## Code Conventions`) and per-module CLAUDE.md notes is followed

   When a Go idiom and a Python internal shape conflict, choose the Go idiom and note the deliberate divergence. A faithful translation of Python internals into Go that violates these idioms is a Gate-2 failure even if Gate 1 passes.

   ### Cross-cutting concerns

   - **No regressions** - do existing tests still pass? Are new tests correct and comprehensive? Do tests use the actual wire format (not a shape that matches a bug)?

   - **Code quality** - KISS/YAGNI/DRY, no dead code, no over-engineering, no premature abstraction.

   - **Comment hygiene** - source comments describe current Go behavior only. Parity tracking lives in `docs/tracking/`, decisioning history and rationale in git log + PR descriptions, never inline. Linter directives (`//nolint:*`, `//go:noinline`, `//go:build`) are self-documenting — adding a comment to explain *why* they exist is redundant; if the suppression is non-obvious, the right fix is to remove the need for the suppression (extract a helper, restructure the code), not to apologize for it. Flag every instance of:
     - **XREF**: "Matches Python SDK's X exactly", "Python SDK: types.py:NNN", or any cross-SDK comparison claim on a Go identifier
     - **HIST**: "Added in Python SDK PR #N", "(PR #N fix)", "(Issue #N)" trailers in source comments
     - **BANNER**: ASCII separators (`// =====...`, `// -----...`) — they cluster Python-source-coordinate references and rot when the file reorganizes
     - **T-PREFIX**: `T###:` task-tracker prefixes on test functions or block comments
     - **FOLLOWING-X**: "following client_test.go patterns" or "following established patterns" XREFs
     - **TUTORIAL**: tutorial-style prose in non-example production code (acceptable in `examples/` and `doc.go` package overview only)
     - **INTER-SDK in doc.go**: phrases like "100% feature parity with the Python SDK" in the package overview comment — describe the Go package, not its relationship to other SDKs
     - **DEAD-REF**: comments naming a function, file, or flag that no longer exists, was renamed, or never landed
     - **GODOC-FORM**: exported-identifier doc comments that don't start with the identifier name
     - **GODOC-LONG**: multi-paragraph doc comments on individual identifiers (only the `doc.go` package overview is exempt)
     - **NOLINT-WHY**: comments that explain a linter suppression — `//nolint:gocyclo // exceeds threshold for table-driven tests`, `// Suppressed because legacy callers...`, or a standalone comment immediately preceding a `//nolint:*` directive that paraphrases what the directive already says. The directive name (`gocyclo`, `funlen`, `revive`) already names the rule; the suppression itself is the explanation. If the *why* is non-trivial, push it into the commit message or eliminate the suppression entirely
     - **RATIONALE**: decision-history prose in source — "we chose X over Y because Z", "preferred map over slice for lookup speed", "decided to use mutex here instead of channel". Belongs in the commit message and PR description, not in the code. The current shape of the code IS the decision; readers don't need the path that got there
     - **EXTRACTION-HIST**: past-tense extraction or refactor history — "Extracted from foo() to keep bar under gocyclo budget", "Pulled out of the dispatch switch", "Was originally inlined in Connect()". Describe the helper's current role, not how it got there. If keeping it extracted is a load-bearing architectural invariant, name the invariant in present tense ("Kept standalone so the dispatch switch stays under gocyclo budget") — never narrate the refactor

     Sweep commands to run before invoking the agent:
     ```bash
     # XREF / FOLLOWING-X
     rg -n '^\s*//.*(Matches Python SDK|Python SDK.*exactly|Added in Python SDK PR|following.*patterns)' --type go
     # BANNER
     rg -n '^\s*//\s*={5,}|^\s*//\s*-{5,}' --type go
     # HIST
     rg -n '^\s*//.*\b(?:PR|Issue) #\d+' --type go
     # T-PREFIX
     rg -n '^\s*//\s*T\d+:' --type go
     # NOLINT-WHY: nolint with inline trailing explanation
     rg -n '//nolint:[A-Za-z,]+\s+//' --type go
     # NOLINT-WHY: standalone comment immediately preceding a nolint that paraphrases it
     rg -nB1 '//nolint:' --type go | rg -i 'exceeds|complexity|threshold|to satisfy|because|allow|suppress'
     # RATIONALE: decision-history prose
     rg -n '^\s*//.*\b(we chose|we decided|we went with|chose .* over|preferred .* over|decided to use)' --type go
     # EXTRACTION-HIST: past-tense extraction narration
     rg -n '^\s*//.*\b(Extracted from|Pulled out of|Moved (out )?of|Refactored to|Was (originally|previously) inlined|Used to be|Lifted from)' --type go
     # EXTRACTION-HIST: "to reduce/keep X under" rationale that often pairs with extraction
     rg -n '^\s*//.*\bto (keep|reduce|stay under|satisfy)\b.*\b(complexity|gocyclo|cyclomatic|budget|funlen|lint)\b' --type go
     ```
     Each hit should appear as a finding in the output unless it is load-bearing (documents a non-obvious why, a hidden constraint, or a wire-format detail not visible from the code alone). Be conservative on "load-bearing" — when the same information exists in `docs/tracking/`, the commit message, or the PR description, the comment is redundant.

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

   COMMENT HYGIENE:
     XREF:            [file:line - cross-SDK comparison claim, current text, recommended rewrite]
     HIST:            [file:line - PR/issue trailer or decisioning history in source]
     BANNER:          [file:line - ASCII separator block]
     T-PREFIX:        [file:line - T### task-tracker prefix]
     FOLLOWING:       [file:line - "following X patterns" XREF]
     TUTORIAL:        [file:line - tutorial prose in production code]
     DEAD-REF:        [file:line - reference to removed/renamed identifier]
     GODOC:           [file:line - bad godoc form or overly long doc on identifier]
     NOLINT-WHY:      [file:line - redundant explanation of a //nolint:* directive; recommend removing the comment or eliminating the suppression]
     RATIONALE:       [file:line - decision-history prose ("we chose X over Y because"); recommend removing and moving rationale to commit message]
     EXTRACTION-HIST: [file:line - past-tense extraction/refactor narration; recommend rewriting in present tense as a load-bearing invariant, or removing]

   VERDICT: [ship / do not ship + one sentence why]
   ```

   Each finding must include file:line. For each blocker: what the Python SDK does, what the Go code does, and the exact fix needed (describe the fix - do not apply it). For each `DOCS RECONCILIATION` entry: name both the doc file:line that is wrong AND the implementation file:line that proves it. For each `COMMENT HYGIENE` entry: quote the current comment (≤80 chars) and give the recommended rewrite or "remove" — these are review-time fixes, the same patterns that get caught by the sweep commands in Step 4.

## Step 5: Present Results

Relay the grumpy-gopher findings directly. Do not summarize or soften. If there are blockers, state them first and clearly.

After presenting findings, note:
- Which tracker items are fully implemented and ready
- Which tracker items are missing or incomplete
- Which examples need to be added or updated for functional live testing
- **Which docs/tracking entries need reconciliation** (tracker rows, CLAUDE.md notes, in-code comments, auto-memory) — distinguish "doc is just imprecise" from "implementation chose a superior approach and docs should record that as a deliberate divergence"
- **Which comment-hygiene patterns appeared in the new code** — XREF/HIST/BANNER/T-PREFIX/etc. are easy to fix as a small follow-up commit, and they compound across branches if left in. A clean branch has zero hits on the Step 4 sweep commands.
- Whether the branch is ready to PR or needs more work

**Stop here. Do not apply any fixes.** The user will decide what to address based on the findings.

## Parity Gates

This branch must pass **both** of the following gates. They are co-equal — a failure on either is a blocker. Passing one does not compensate for failing the other.

### Gate 1 — Observable-behavior parity with Python SDK

What Python users perceive must match what Go users perceive:
- **Wire format**: JSON field names, constant values, message shapes, CLI flag names and values
- **Public API surface**: method names, parameter names/types, return types, error types, optional vs required field discrimination
- **Semantics**: same inputs produce equivalent outputs; same edge cases (nil/None, empty, malformed) produce equivalent behavior; same config knobs (env vars, options) have equivalent effects

### Gate 2 — Idiomatic Go delivery

How the parity contract is delivered must look like Go a senior Go reviewer would write:
- **Context-first**, `fmt.Errorf` with `%w`, nil-safety, small focused interfaces, no unnecessary exports
- **gofmt -s clean**, golangci-lint clean, gocyclo under 15 — extract helpers rather than suppressing with `//nolint:gocyclo`
- **Channel-based concurrency**, bounded goroutine lifetimes, no leaks
- **Zero-value usability**, constructors only when invariants demand them
- **Repo conventions** from CLAUDE.md (`## Detected Patterns`, `## Code Conventions`) and per-module CLAUDE.md notes

### Mirroring Python internals is NOT a parity goal

Parity on **internal mechanics** is not a goal — only the observable contract is. Since this is Go, prefer:
- Idiomatic Go over mirroring Python internals (e.g. functional options vs keyword-only args, sealed interface vs union types, channels vs asyncio primitives)
- Established repo patterns (see CLAUDE.md `## Detected Patterns`) over Python-shaped code
- A superior Go pattern over an existing repo pattern only when the improvement is clear and non-disruptive

When a Go idiom and a Python internal shape conflict, choose the Go idiom and note the deliberate divergence in the PR description (not in source comments). A faithful translation of Python internals into Go that violates Gate 2 fails review even if Gate 1 passes.

## Docs as a derived artifact

Implementation is the source of truth on a merged or review-ready branch — tracker prose, CLAUDE.md notes, in-code comments, and auto-memory entries are derived artifacts that must follow the implementation, not the other way around. When the reviewer finds doc/code mismatch:

- **The implementation is right by default.** It was reviewed; the docs may have been written from memory, copied from an earlier draft, or left untouched when the implementation evolved.
- **Superior-implementation drift is real.** When the implementation deliberately diverges from the tracker spec or Python SDK to be better-than-Python (clearer naming, idiomatic Go, broader API surface, stronger invariants), the docs MUST record this as an intentional divergence with a one-line rationale — silent superiority becomes invisible to future contributors and they may "fix" it back.
- **Imprecise tracker prose is a common pitfall.** Tracker rows often describe Python behavior in summary form (e.g., "subtype `get_mcp_status`") that doesn't survive grep against the actual Python wire string. The reviewer must check the wire string, not the tracker prose.
- **Auto-memory and CLAUDE.md drift compounds.** A wrong note in `internal/<module>/CLAUDE.md` gets re-rendered into project root CLAUDE.md and into auto-memory entries. Fix at the root cause (the module CLAUDE.md or the source comment), not at every downstream copy.

The reviewer reports these under `DOCS RECONCILIATION` so the user can fix docs before opening the PR (or as a follow-up cleanup commit), without conflating docs polish with code blockers.
