# Python SDK PR Replay Tracker

Tracks all Python SDK PRs that need to be replayed into the Go SDK to restore parity. Replay top to bottom within each phase.

## Snapshot

| Field | Value |
|:------|:------|
| Snapshot date | April 12, 2026 |
| Coverage window | Jan 6, 2026 - April 12, 2026 |
| Parity milestone | Go PR #77 (Jan 6, 2026) - formal parity documentation |
| Last ported feature | Go PR #99 (Jan 24, 2026) - tool_use_result (Python PR #495) |
| Python SDK at checkpoint | v0.1.22 |
| Python SDK at snapshot | v0.1.58 |
| Go status verified | rows #9 and #13 against Go PR #PRNUM; rows #7, #8, #15, #42 and #47 against Go v0.9.0 (cf58dbc, Oct 5, 2026); rows #10 and #11 against Go main de83b7d (Oct 4, 2026); rows #12, #16, #17, #20 and #31 against Go v0.8.0 (6c9d76f, Oct 4, 2026); rows #6, #14, #18, #29 and #43 against Go main 58cf570 (v0.7.1, Oct 1, 2026); rows #14a and #30 against Go PR #163 |

PRs merged after April 12, 2026 do not belong in this file. They are tracked in [post-snapshot.md](post-snapshot.md), which keeps the snapshot above stable as the canonical Phase 1-4 record.

## Summary

| Category | Count |
|:---------|:------|
| Actionable | 49 |
| Skip (CI/CD, Python-specific, docs-only) | 31 |

---

## Phase 1 - Jan 26 to Feb 20

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| - | #495 | tool_use_result on UserMessage | Jan 23 | feat | done | #99 | Added ToolUseResult map[string]any field to UserMessage with HasToolUseResult()/GetToolUseResult() helpers |
| 1 | #516 | get_mcp_status() method | Jan 26 | feat | done | #124 | Python added get_mcp_status() returning McpStatusResponse with list of McpServerStatus (name, status, serverInfo, error, config, scope, tools). Add GetMcpStatus(ctx) to Client interface, control request subtype `"mcp_status"` (wire string from `_internal/query.py`), McpServerStatus/McpStatusResponse types in `internal/control/types.go`, wire through Transport. |
| 2 | #535 | PostToolUseFailure hook event | Jan 30 | feat | done | #125 | `HookEventPostToolUseFailure` constant + `PostToolUseFailureHookInput` (BaseHookInput + ToolName/ToolInput/ToolUseID/Error/IsInterrupt) + `PostToolUseFailureHookSpecificOutput` in `internal/control/types_hook.go`; `parseHookInput` case + `getBoolPtr` helper in `hooks.go`; re-exports in `options.go`. `_SubagentContextMixin` fields (`agent_id`/`agent_type`) deferred to item #13 (Python PR #628), which is where Python actually introduces the mixin and applies it to PostToolUseFailureHookInput. |
| 3 | #506 | Properly populate AssistantMessage error field | Feb 3 | fix | done | #124 | Parser fix in `internal/parser/json.go`: read `error` from top-level `data["error"]` instead of nested `data["message"]["error"]`; tests in `internal/parser/json_test.go` updated to correct wire format with `billing_error`/`server_error` cases + regression test verifying nested error is ignored. Landed in PR #124's squash alongside item #1. |
| 4 | #545 | Missing hook events + fix fields | Feb 3 | feat | done | #128 | Python added 3 hook events (`Notification`, `SubagentStart`, `PermissionRequest`) each with HookInput + HookSpecificOutput types. Also fixed missing fields on existing hooks: `tool_use_id` on PreToolUseHookInput and PostToolUseHookInput; `agent_id`/`agent_transcript_path`/`agent_type` as flat required fields on SubagentStopHookInput (NOT via a mixin - the mixin lands separately in item #13 / PR #628); `additionalContext` on PreToolUseHookSpecificOutput; `updatedMCPToolOutput` on PostToolUseHookSpecificOutput. Hook event count: 7 -> 10. Added `getAnySlice` helper in `internal/control/hooks.go`. `PermissionRequestHookSpecificOutput.Decision` is required (no `omitempty`). |
| 5 | #551 | MCP tool annotations | Feb 5 | feat | done | #133 | Python added optional `annotations` parameter to `@tool` decorator / `SdkMcpTool` accepting `mcp.types.ToolAnnotations` (title, readOnlyHint, destructiveHint, idempotentHint, openWorldHint). Wired into the JSONRPC `tools/list` response in `_internal/query.py` (omitted when None, `model_dump(exclude_none=True)` when set). Re-exports `ToolAnnotations` from the top-level package. Add authoring-side `ToolAnnotations` struct in `internal/shared/options.go`, `Annotations` field on `McpToolDefinition`, `ToolOption`/`WithToolAnnotations` on `NewTool()` in `mcp.go`, conditional `annotations` key in `routeMcpMethod` tools/list branch in `internal/control/mcp.go` (json round-trip honors `omitempty`). NOT to be confused with the CLI-status-side `McpToolAnnotations` (readOnly/destructive/openWorld) attached to `McpServerStatus.Tools`, which was already wired in Phase1 #1 (Go PR #124, Python PR #516). |
| 6 | #468 | Send agent definitions via initialize request | Feb 5 | feat | done | #134 | Python sends agents in the initialize control request instead of via --agents CLI flag. Go change goes beyond the literal field move: bundles the Query→streaming migration (issue #113) so both `Query()` and `Client` paths run unconditional `initialize` over the control protocol. Adds `InitializeRequest.Agents map[string]any` (+ `WithAgents` ProtocolOption) in `internal/control/`; removes `--agents`/`--print` and the `closeStdin`/`promptArg`/`NewWithPrompt` constructors from `internal/cli/` and `internal/subprocess/`; adds `Transport.EndInput(ctx)` (stdin-only close). `Query()` writes the prompt as a `{"type":"user","message":{...}}` JSON line and calls `EndInput` either right after the write (no bidirectional needs) or after the first `ResultMessage` (hooks/CanUseTool/SDK-MCP/checkpointing). Subprocess converts `shared.AgentDefinition` → `map[string]any` at the boundary (`agentsToMap`) so the `control` package keeps no `shared` dependency. |
| 7 | #565 | ThinkingConfig types and effort option | Feb 11 | feat | done | #136, #181 | Python replaced max_thinking_tokens with the ThinkingConfig union: ThinkingConfigAdaptive (type="adaptive"), ThinkingConfigEnabled (type="enabled", budget_tokens), ThinkingConfigDisabled (type="disabled"), and added the effort option. CLI flags (`subprocess_cli.py`): adaptive -> `--thinking adaptive`, enabled -> `--max-thinking-tokens N`, disabled -> `--thinking disabled`; `thinking` takes precedence over `max_thinking_tokens`. **Go PR #136**: `WithEffort()` and `--effort`. **Go PR #181**: sealed `ThinkingConfig` interface with `ThinkingConfigAdaptive`, `ThinkingConfigEnabled` and `ThinkingConfigDisabled` in `internal/shared/options.go`, `WithThinking()`, `addThinkingFlags` in `internal/cli/discovery.go`, `WithMaxThinkingTokens()` deprecated and now sends `--max-thinking-tokens` (the flag was commented out before, so the option did nothing). `NewOptions()` no longer sets a default of 8000. Divergence: Go treats `MaxThinkingTokens` 0 as unset (Python sends `--max-thinking-tokens 0`); use `ThinkingConfigDisabled` to turn thinking off. |
| 8 | #598 | Handle unknown message types gracefully | Feb 20 | fix | done | #182 | Python `_internal/message_parser.py` skips an unknown top-level `type` (returns None, debug log; the callers skip None) and has no default case for content blocks, so an unknown block is dropped. Go `ParseMessage` in `internal/parser/json.go` now returns a nil message and a nil error for an unknown type (`ProcessLine` already drops nil), and `parseContentBlocks` drops a block of an unknown type. A known type with a missing required field still returns a `MessageParseError`, as in Python. |

---

## Phase 2 - Mar 3 to Mar 16

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 9 | #619 | stop_reason on ResultMessage | Mar 3 | feat | done | #PRNUM | Python added `stop_reason: Optional[str] = None` to ResultMessage, read with `data.get("stop_reason")`. Go adds `StopReason *string` with `json:"stop_reason,omitempty"` to `ResultMessage` in `internal/shared/message.go` (pointer, not the `string` this row first suggested, so an absent key and `null` both stay nil as in Python), set in `parseResultOptionalFields` in `internal/parser/json.go`. `AssistantMessage.stop_reason` is row #24. |
| 10 | #620 | MCP control methods + typed McpServerStatus | Mar 3 | feat | partial | #168 | Python added reconnect_mcp_server(name) and toggle_mcp_server(name, enabled). Also typed McpServerStatus with McpServerConnectionStatus enum ("connected"/"failed"/"needs-auth"/"pending"/"disabled") and McpServerInfo (name, version). Add ReconnectMcpServer(ctx, name) and ToggleMcpServer(ctx, name, enabled) on Client interface, new control request subtypes, wire through Transport. The same PR also added `stop_task(task_id)` (Python commit 28f9b4b: `client.py` `stop_task`, `_internal/query.py` `stop_task` control request) to stop one running task. **Partial scope landed in Go PR #168**: `StopTask(ctx, taskID)` on `Client` and `Transport`, sending `{"subtype":"stop_task","task_id":...}` (`StopTaskRequest`, `SubtypeStopTask` in `internal/control/types.go`); returns "client not connected" before `Connect()`. The typed `McpServerStatus`, `McpServerConnectionStatus` and `McpServerInfo` already landed in Go PR #124 with `GetMcpStatus` (row #1). **Deferred**: (a) `ReconnectMcpServer(ctx, name)` (`reconnect_mcp_server` control request); (b) `ToggleMcpServer(ctx, name, enabled)` (`toggle_mcp_server` control request). |
| 11 | #621 | Typed TaskStarted/TaskProgress/TaskNotification | Mar 3 | feat | done | #168 | Python created typed SystemMessage subclasses that keep `subtype`/`data`: TaskStartedMessage (task_id, description, uuid, session_id; optional tool_use_id, task_type), TaskProgressMessage (task_id, description, usage, uuid, session_id; optional tool_use_id, last_tool_name), TaskNotificationMessage (task_id, status, output_file, summary, uuid, session_id; optional tool_use_id, usage), plus TaskUsage (total_tokens, tool_uses, duration_ms) and TaskNotificationStatus ("completed"/"failed"/"stopped"). A missing required field raises MessageParseError. Landed in Go PR #168 in `internal/shared/task.go`: the messages still arrive as `*SystemMessage` (so a `case *SystemMessage` switch keeps seeing them), and `AsTaskStarted()`/`AsTaskProgress()`/`AsTaskNotification()` return typed structs that embed `SystemMessage`. Required fields are values, optional ones pointers; `parseSystemMessage` returns a `MessageParseError` when a required field is missing, as Python does. CLI fields Python does not model (`subagent_type`, `is_backgrounded`, `summary`, ...) stay in `Data`. Go issue #143. |
| 12 | #622 | list_sessions / get_session_messages | Mar 3 | feat | done | #109, #171 | Python added top-level list_sessions() and get_session_messages(session_id) using CLI flags --list-sessions and --get-session-messages. Returns []SDKSessionInfo and []SessionMessage. Add package-level functions, SDKSessionInfo struct (session_id, summary, last_modified, file_size, custom_title, first_prompt, git_branch, cwd), SessionMessage struct (type, uuid, session_id, message, parent_tool_use_id). Standalone CLI invocations, not control protocol. Landed in Go PR #109 as `ListSessions()` and `GetSessionMessages()` (read the session JSONL files directly, as Python does). Long-path gap fixed with rows 16/17: `encodeCwd` cuts names over 200 chars and adds `-<simpleHash>` (`sessions.py` `_sanitize_path`); `projectDirsForOpts` resolves symlinks (`_canonicalize_path`) and falls back to the first dir with the same 200-char prefix (`_find_project_dir`). Known gap: Python also applies NFC to the path; Go does not, because `golang.org/x/text` v0.22.0 (the newest with `go 1.18`) has GO-2026-5970 and the fix needs go 1.25. |
| 13 | #628 | agent_id/agent_type in hook inputs | Mar 3 | feat | done | #PRNUM | Python introduced `_SubagentContextMixin` (TypedDict, total=False) and applied it to the four tool-lifecycle hook inputs the CLI populates: PreToolUseHookInput, PostToolUseHookInput, PostToolUseFailureHookInput, PermissionRequestHookInput. `agent_id` is present only inside a sub-agent; `agent_type` is present inside a sub-agent or on the main thread of a session started with `--agent`. Go adds `AgentID *string` and `AgentType *string` (with `omitempty`) to the four structs in `internal/control/types_hook.go` as plain fields (no embedded mixin struct, so composite literals stay flat), parsed with `getStringPtr` in `parseHookInput`. `SubagentStart`/`SubagentStop` inputs keep their required `string` fields. |
| 14 | #630 | Fix string prompt closing stdin before MCP init | Mar 4 | fix | done | #134 | Python fixed race: string prompt closed stdin before SDK MCP servers initialized. Go: `Query()` keeps stdin open until `Next()` sees the first `ResultMessage` when hooks, `CanUseTool`, SDK MCP servers or file checkpointing need the control protocol (`needsBidirectionalStdin` and `maybeEndInputAfterResult` in `query.go`, pinned by `TestNeedsBidirectionalStdin`). `Client` never closes stdin during a session. |
| 14a | #642 | Wait for graceful subprocess shutdown before SIGTERM | Mar 19 | fix | done | #150, #163 | Python waits up to 5s for the CLI to exit after stdin EOF, then sends SIGTERM (`subprocess_cli.py` `close()`). Go #163: `teardownLocked` closes stdin, then `terminateProcess` waits up to 5s on `processDone` before SIGTERM. `handleStdout` keeps draining stdout while `closing` is set, and `t.ctx` is cancelled only after the process ends, so CLI output during shutdown no longer ends the grace period. `TestCloseWaitsForExitAfterEOF` covers a CLI that writes output and exits 1.5s after EOF without a signal. |
| 15 | #648 | Typed RateLimitEvent message | Mar 12 | feat | done | #129, #187, #189 | Python added the RateLimitEvent message with RateLimitInfo: status, resets_at, rate_limit_type, utilization, overage_status, overage_resets_at, overage_disabled_reason, raw, plus uuid and session_id. **Go PR #129**: `RateLimitEventMessage`, `RateLimitInfo`, parser case, re-exports. **Go PR #187** (contributor): `RateLimitInfo.Raw`. **Go PR #189**: `Utilization *float64` (nil when absent), `OverageDisabledReason`, `RateLimitStatusAllowedWarning`/`RateLimitStatusRejected`, the five `RateLimitType*` constants, the issue trailer removed from `internal/shared/message.go`, and the tests use `rejected` (not the invalid `blocked`) with checked type assertions. Deliberate divergences: Go accepts a message without `uuid` or `session_id` (Python raises `MessageParseError`), so a CLI heartbeat without them does not become a stream error; Go keeps `IsUsingOverage` (Python reads it only from raw) because removing it would break callers. |
| 16 | #668 | rename_session | Mar 12 | feat | done | #171 | Python `rename_session(session_id, title, directory=None)` appends `{"type":"custom-title","customTitle":...,"sessionId":...}` to the session JSONL (no CLI flag, no control request; `session_mutations.py`). Go: `RenameSession(sessionID, title string, opts ...SessionOption) error` in `internal/session/session.go`. UUID check, trimmed title must be non-empty, append with `O_WRONLY|O_APPEND` and no create; an empty (0-byte) file is a stub and the search goes on; not found wraps `errSessionNotFound`. The directory lookup has the NFC gap that row 12 records. |
| 17 | #670 | tag_session with Unicode sanitization | Mar 12 | feat | partial | #171 | Python `tag_session(session_id, tag, directory=None)` appends `{"type":"tag","tag":...,"sessionId":...}`; `None` writes `""`, which readers treat as cleared. The tag gets up to 10 passes of NFKC plus removal of Cf, Co and Cn runes and explicit zero-width, directional, BOM and private-use ranges (`_sanitize_unicode`). Go: `TagSession(sessionID string, tag *string, opts ...SessionOption) error`, nil clears; `sanitizeUnicode` removes the same Cf, Co and Cn runes and ranges in one pass. Known gap: Go does not apply NFKC (for example fullwidth "A" stays as is), because `golang.org/x/text` v0.22.0 (the newest with `go 1.18`) has GO-2026-5970 and the fix needs go 1.25. Revisit when the module minimum Go version is 1.25. |
| 18 | #685 | Preserve per-turn usage on AssistantMessage | Mar 16 | feat | done | #148 | Python added `usage: dict` to AssistantMessage for per-turn token stats (input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens). Added `Usage *map[string]any` with `json:"usage,omitempty"` to AssistantMessage in `internal/shared/message.go` (pointer form to mirror the existing `ResultMessage.Usage` idiom, not the plain `map[string]any` this row originally suggested). Parsed from the nested `data["message"]["usage"]`, not top-level `data["usage"]` (that path is `ResultMessage`'s). Python's own PR #685 ships usage alone, without a message id - `message_id`/`stop_reason`/`session_id`/`uuid` land together in a later, separate Python PR (#718), already tracked at row #24 in this file; deliberately not pulled forward here to keep this replay 1:1 with its source PR. |
| 19 | #684 | skills/memory/mcpServers on AgentDefinition | Mar 16 | feat | pending | - | Python added skills (list), memory (config), mcpServers (map) to AgentDefinition. Add `Skills []any`, `Memory any`, `McpServers map[string]McpServerConfig` with JSON tags to AgentDefinition in `internal/shared/options.go`. |
| 20 | #686 | default-if-absent for CLAUDE_CODE_ENTRYPOINT | Mar 16 | fix | done | #170 | Python sets `CLAUDE_CODE_ENTRYPOINT` after the inherited env, so it replaces an inherited value, and `options.env` can override it (`subprocess_cli.py` `connect()`, `test_caller_can_override_entrypoint`). Go `buildEnvironment` in `internal/subprocess/config.go` already had the same effective order (exec uses the last duplicate key); `TestSubprocessEnvironmentVariables` cases `inherited_entrypoint_replaced` and `extra_env_entrypoint_wins` pin it. |

---

## Phase 3 - Mar 20 to Mar 30

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 21 | #667 | tag/created_at on SDKSessionInfo + get_session_info | Mar 20 | feat | done | #109 | Python added tag and created_at fields to SDKSessionInfo. Added get_session_info(session_id) function. Add `Tag *string`, `CreatedAt *string` to SDKSessionInfo. Add GetSessionInfo(ctx, sessionID) package-level function. Landed in Go PR #109: `GetSessionInfo()`, plus `Tag *string` and `CreatedAt *int64` on SDKSessionInfo. |
| 22 | #591 | SystemPromptFile support | Mar 25 | feat | pending | - | Python added SystemPromptFile TypedDict passing --system-prompt-file to CLI. Add SystemPromptFile struct (Path string), update system_prompt option handling, add --system-prompt-file flag in `internal/cli/command.go`. |
| 23 | #717 | propagate is_error from SDK MCP tool results | Mar 25 | fix | pending | - | Python fixed is_error flag propagation from McpToolResult to CLI. Check `internal/control/mcp.go` - verify IsError on McpToolResult is included in JSON response to CLI. |
| 24 | #718 | Preserve dropped fields on AssistantMessage/ResultMessage | Mar 25 | feat | pending | - | Python added typed optional fields that the parser used to drop: `AssistantMessage.message_id`, `stop_reason` (from `data["message"]`), `session_id`, `uuid`, and `ResultMessage.model_usage`, `permission_denials`, `uuid` (all optional, default `None`). Add the same fields to Go `AssistantMessage` and `ResultMessage` in `internal/shared/message.go` as pointers or nil slices/maps, and parse them in `internal/parser/json.go`. (This row first described an `extra_fields` catch-all; #718 has no such field.) |
| 25 | #719 | add missing dontAsk permission mode | Mar 25 | fix | pending | - | Python added "dontAsk" to PermissionMode (was missing despite being valid CLI mode). Add `PermissionModeDontAsk PermissionMode = "dontAsk"`. Check if Go SDK already has this. |
| 26 | #720 | remove duplicate version warning | Mar 25 | fix | pending | - | Python removed duplicate CLI version warning. Check `internal/cli/version.go` - verify warning fires only once per session. |
| 27 | #723 | skip non-JSON lines on CLI stdout | Mar 25 | fix | pending | - | Python skips non-JSON debug lines on stdout (DEBUG env in Docker/Lambda). Check `internal/parser/parser.go` - verify graceful skip of lines not starting with `{`. May already be handled by speculative parsing. |
| 28 | #725 | handle resource_link/embedded resource in SDK MCP | Mar 25 | fix | pending | - | Python added resource_link and embedded_resource content types in SDK MCP tool responses. Check `mcp.go` - add support for these content types in McpContent or pass through as-is. |
| 29 | #731 | remove stdin timeout for hooks/SDK MCP | Mar 25 | fix | done | #134 | Python removed the timeout on the wait for the first result before it closes stdin (`wait_for_result_and_end_input`, `query.py`). Go has no such timeout: `maybeEndInputAfterResult` in `query.go` closes stdin only when the first `ResultMessage` arrives, and the iterator closes the transport on every terminal path (#153). |
| 30 | #729 | SIGKILL fallback when SIGTERM blocks | Mar 26 | fix | done | #150, #163 | Python waits 5s after SIGTERM, then sends SIGKILL. Go #163: `terminateProcess` uses fixed timers and never reads `t.ctx`, so the sequence is EOF, 5s, SIGTERM, 5s, SIGKILL, 5s (bounded). On Windows SIGTERM fails and the sequence goes to Kill (Python `terminate()` is TerminateProcess there). `TestCloseSIGTERMGraceBeforeKill` checks the lower bounds with a mock CLI that ignores SIGTERM. |
| 31 | #732 | filter CLAUDECODE env var from subprocess | Mar 26 | fix | done | #170 | Python drops the exact key `CLAUDECODE` from the inherited env; `options.env` can set it again. Go `buildEnvironment` in `internal/subprocess/config.go` now skips inherited `CLAUDECODE=` entries before it adds `ExtraEnv` (Go issue #102). |
| 32 | #744 | fork_session, delete_session, offset pagination | Mar 26 | feat | pending | - | Python added fork_session(session_id) -> ForkSessionResult (new_session_id), delete_session(session_id), offset/limit params on list_sessions(). Add ForkSession(), DeleteSession() functions, ForkSessionResult struct, update ListSessions with pagination. |
| 33 | #747 | task_budget option | Mar 26 | feat | pending | - | Python added TaskBudget TypedDict (total int) and task_budget option. Passed as --task-budget CLI flag. Add TaskBudget struct, WithTaskBudget() in `options.go`, CLI flag in `internal/cli/`. |
| 34 | #743 | pass initialize_timeout from env var | Mar 26 | fix | pending | - | Python reads CLAUDE_CODE_STREAM_CLOSE_TIMEOUT env var to override initialize timeout. Check if Go SDK respects this, add support if not. |
| 35 | #749 | errors field on ResultMessage | Mar 27 | fix | done | #114 | Python added `errors: list[str]` to ResultMessage for non-fatal session errors. Add `Errors []string` with `json:"errors,omitempty"` to ResultMessage. Distinct from IsError (fatal failure flag). |
| 36 | #759 | disallowedTools/maxTurns/initialPrompt on AgentDefinition | Mar 27 | feat | pending | - | Python added disallowedTools ([]string), maxTurns (int), initialPrompt (string) to AgentDefinition. Add `DisallowedTools []string`, `MaxTurns *int`, `InitialPrompt *string` with JSON tags to AgentDefinition. |
| 37 | #750 | session_id on ClaudeAgentOptions | Mar 28 | feat | pending | - | Python added session_id option for custom session IDs. Passed as --session-id CLI flag. Add WithSessionID(id string) option, `SessionID *string` to Options, CLI flag in `internal/cli/`. |
| 38 | #754 | tool_use_id/agent_id in ToolPermissionContext | Mar 28 | feat | pending | - | Python added tool_use_id and agent_id to ToolPermissionContext for richer permission callback context. Add `ToolUseID string` and `AgentID string` to ToolPermissionContext in `internal/control/types.go`. |
| 39 | #764 | get_context_usage() | Mar 28 | feat | pending | - | Python added get_context_usage() returning ContextUsageResponse: categories ([]ContextUsageCategory with name/tokens/color/isDeferred), totalTokens, maxTokens, percentage, model, optional autoCompactThreshold/mcpTools/agents. Add GetContextUsage(ctx) to Client, ContextUsageResponse/ContextUsageCategory structs, control request subtype, wire through Transport. |
| 40 | #751 | control_cancel_request handling | Mar 28 | feat | pending | - | Python handles control_cancel_request from CLI to cancel pending control requests (e.g., timed-out permission callbacks). Update `internal/control/protocol.go` to handle incoming "cancel_request" by canceling context of pending request by request_id. |
| 41 | #769 | send string prompt in connect() | Mar 28 | fix | pending | - | Python fixed connect(prompt="...") silently dropping prompt. Verify Go Client.Connect() sends prompt via stdin after connection established. |
| 42 | #778 | omit --setting-sources when empty | Mar 30 | fix | n/a | #170 | Python #778 omitted `--setting-sources` for an empty list. Post-snapshot P5 (Python #822) reversed this: Python now emits `--setting-sources=` for an empty list and omits the flag only for None. Go follows P5 (`addSessionFlags` in `internal/cli/discovery.go`, Go PR #170), so this row is superseded. |
| 43 | #780 | background task for string prompts with hooks/MCP | Mar 30 | fix | done | #134 | Python made the first-result wait a background task so it cannot deadlock a string prompt with hooks or SDK MCP servers. Go never blocks on that wait: `Next()` calls `maybeEndInputAfterResult` as it returns each message, and control requests run on their own goroutines (#151). |

---

## Phase 4 - Mar 31 to Apr 8

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 44 | #782 | background/effort/permissionMode on AgentDefinition | Mar 31 | feat | pending | - | Python added background (bool), effort (string), permissionMode (PermissionMode) to AgentDefinition. Add `Background *bool`, `Effort *string`, `PermissionMode *PermissionMode` with JSON tags. |
| 45 | #756 | forward maxResultSizeChars via _meta | Apr 2 | fix | pending | - | Python forwards maxResultSizeChars in _meta of MCP tool results for large results (>50K chars). Add _meta.maxResultSizeChars to tool result JSON when exceeding threshold. |
| 46 | #785 | 'auto' PermissionMode | Apr 7 | feat | pending | - | Python added "auto" PermissionMode (auto-approves safe tools, prompts for dangerous). Add `PermissionModeAuto PermissionMode = "auto"` in `internal/shared/options.go`. |
| 47 | #796 | --thinking flag for adaptive/disabled | Apr 7 | fix | done | #181 | Python fixed the CLI flags: adaptive -> `--thinking adaptive`, disabled -> `--thinking disabled` (not budget tokens). Go emits these from `ThinkingConfigAdaptive` and `ThinkingConfigDisabled` (`thinkingArgs` in `internal/shared/options.go`, `addThinkingFlags` in `internal/cli/discovery.go`), together with row 7. |
| 48 | #797 | exclude_dynamic_sections on SystemPromptPreset | Apr 8 | feat | pending | - | Python added exclude_dynamic_sections bool to SystemPromptPreset for cross-user prompt caching. Add ExcludeDynamicSections bool to SystemPromptPreset struct (create if needed), pass as CLI flag. |

---

## Skipped PRs

Not applicable to Go SDK. Listed for completeness so nothing falls through cracks.

| Py PR | Title | Merged | Reason |
|:------|:------|:-------|:-------|
| #451 | Skip jobs requiring secrets from forks | Jan 5 | CI workflow |
| #442 | Update Claude Agent SDK documentation link | Jan 8 | Docs-only |
| #465 | Release v0.1.19 | Jan 8 | Python release |
| #467 | Update claude-code actions to @v1 | Jan 12 | CI workflow |
| #485 | Make permission callback e2e test robust | Jan 16 | Python e2e test |
| #486 | Release v0.1.20 | Jan 16 | Python release |
| #488 | Extract build-and-publish workflow | Jan 21 | CI workflow |
| #503 | Release v0.1.21 | Jan 21 | Python release |
| #504 | Fix release job permissions | Jan 22 | CI workflow |
| #511 | Fix SSH remote URL in auto-release | Jan 23 | CI workflow |
| #512 | Release v0.1.22 | Jan 23 | Python release |
| #536 | Add debug output to MCP e2e tests | Jan 30 | Python test |
| #537 | Pin CI to Python 3.13 | Jan 30 | CI workflow |
| #538 | Enforce sequential tool execution in MCP e2e | Jan 30 | Python test |
| #539 | Simplify release flow + RELEASING.md | Jan 30 | CI workflow |
| #556 | Update Claude model to opus-4-6 in CI | Feb 7 | CI workflow |
| #644 | Enable fine-grained tool streaming | Mar 6 | Reverted by #671 |
| #661 | Publish macOS x86_64 wheel | Mar 9 | Python wheel |
| #662 | Upload check wheels as artifacts | Mar 12 | Python wheel |
| #649 | Clarify allowed_tools as permission allowlist | Mar 10 | Docs-only |
| #671 | Revert fine-grained tool streaming (#644) | Mar 10 | Revert of #644 |
| #700 | Harden PyPI publish | Mar 20 | PyPI infra |
| #707 | Release v0.1.49 | Mar 20 | Python release |
| #705 | Daily PyPI storage quota monitoring | Mar 20 | PyPI infra |
| #708 | Retry install.sh fetch on 429 | Mar 24 | Build infra |
| #722 | Defer CLI discovery to connect() | Mar 25 | Python async event loop specific |
| #736 | Convert TypedDict input_schema to JSON Schema | Mar 26 | Python TypedDict specific |
| #746 | Resolve cross-task cancel scope RuntimeError | Mar 26 | Python async/anyio specific |
| #760 | Increase test-examples timeout | Mar 28 | CI timeout |
| #761 | typing_extensions.TypedDict on Python 3.10 | Mar 27 | Python 3.10 compat |
| #762 | Annotated for per-parameter descriptions in @tool | Mar 28 | Python typing.Annotated specific |

---

## How to Update

1. **Starting work:** Set Go Status to `in-progress` for the current row
2. **PR merged:** Set Go Status to `done`, fill Go PR column with `#N`
3. **Not applicable:** Set Go Status to `skip`, add reason in Notes
4. **New Python PRs (after April 12, 2026):** Add the row to [post-snapshot.md](post-snapshot.md) using the next available `P` prefix (P1, P2, ...). The snapshot date in this file stays Apr 12, 2026 - it bounds Phases 1-4 as the canonical record.
