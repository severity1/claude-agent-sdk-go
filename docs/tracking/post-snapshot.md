# Python SDK PR Replay Tracker - Post-Snapshot

Tracks Python SDK PRs merged **after** the snapshot date in `README.md` (April 12, 2026). The main tracker is snapshot-bounded by design and is not updated for post-snapshot Python work; this file holds entries for PRs that need to be replayed into the Go SDK from beyond that window.

The schema mirrors `README.md` so rows can be moved between files if the snapshot date is ever rolled forward and a post-snapshot PR retroactively falls inside the new window.

## Snapshot

| Field | Value |
|:------|:------|
| Coverage start | April 13, 2026 |
| Coverage end | rolling - update as new PRs land |
| Companion file | [README.md](README.md) (Jan 6 - Apr 12, 2026 snapshot) |

## Scope rules

Same scope rules as [README.md](README.md): Python-leads-Go only. Go-side bug fixes for existing Go-only behavior do not belong in either file.

---

## Post-Snapshot Entries

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| P1 | #804 | Top-level `skills` option on ClaudeAgentOptions | Apr 17 | feat | partial | #130 | Python added `skills: list[str] \| Literal["all"] \| None` to ClaudeAgentOptions as the single place to enable Skills for the main session, mirroring the existing AgentDefinition.skills field for subagents (PR #684). Two behaviors land together: (1) CLI-flag transformation via `_apply_skills_defaults` in `_internal/transport/subprocess_cli.py` - "all" appends bare `Skill` to `--allowedTools`, list of names appends `Skill(name)` per entry, sets `setting_sources` default to `["user", "project"]` when unset and skills is non-nil; (2) Initialize control-request forwarding in `_internal/query.py` - when skills is a `list[str]`, sent as `request["skills"]` so supporting CLIs filter which Skills load into the system prompt (`"all"` and `None` both omit the field). **Partial scope landed in Go PR #130**: `Skills any` field on Options, SkillsAll constant, WithSkills/WithSkillsAll/WithSkillsList/WithSkillsDisabled helpers, applySkillsDefaults CLI-flag transformation in `internal/cli/discovery.go`, 8-case table-driven test. **Deferred to complete this row**: (a) initialize control-request forwarding - add `Skills *[]string` (pointer-to-slice preserves the empty-list-vs-absent distinction Python's `isinstance(_, list)` check requires) to InitializeRequest in `internal/control/types.go`, plumb from Options.Skills through Protocol constructor, emit only when value is `[]string`; (b) tighten `validateSkillsDisabled` test in `internal/cli/discovery_test.go` to assert `--setting-sources user,project` also fires for the empty-list case (Python `_apply_skills_defaults` triggers the default for `skills=[]`); (c) example coverage demonstrating WithSkillsList/WithSkillsAll (e.g. `examples/04_query_with_tools` or `examples/08_client_advanced`); (d) decide on `WithSkills(any)` runtime validation - it currently silently no-ops on garbage input; either document `WithSkills(any)` as advanced escape-hatch and promote typed helpers, or add runtime panic/err on unsupported types; (e) sweep 5 XREF comments in changed files (`internal/cli/discovery.go:152`, `:483-484`; `internal/shared/options.go:43`, `:195`; `options.go:323`). |
