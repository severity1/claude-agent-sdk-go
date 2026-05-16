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
| 5 | #551 | MCP tool annotations | Feb 5 | feat | pending | - | Python added McpToolAnnotations (readOnly, destructive, openWorld bools) and McpToolInfo (name, description, annotations). Returned in MCP server status and tool listings. Add structs in `mcp.go` or `internal/shared/options.go`. Wire into McpServerStatus.Tools. |
| 6 | #468 | Send agent definitions via initialize request | Feb 5 | feat | pending | - | Python sends agents in initialize control request instead of --agents CLI flag. Add `Agents map[string]AgentDefinition` to InitializeRequest in `internal/control/types.go`. Update `internal/control/protocol.go` to include agents. Remove/deprecate --agents flag in `internal/cli/`. |
| 7 | #565 | ThinkingConfig types and effort option | Feb 11 | feat | pending | - | Python replaced max_thinking_tokens with ThinkingConfig union: ThinkingConfigAdaptive (type="adaptive"), ThinkingConfigEnabled (type="enabled", budget_tokens), ThinkingConfigDisabled (type="disabled"). Added effort option ("low"/"medium"/"high"/"max"). Add ThinkingConfig interface + 3 structs, WithThinking(), WithEffort() in `options.go`. Update CLI flags: adaptive -> `--thinking adaptive`, disabled -> `--thinking disabled`, enabled -> `--thinking-budget N`. Deprecate WithMaxThinkingTokens(). |
| 8 | #598 | Handle unknown message types gracefully | Feb 20 | fix | pending | - | Python returns a raw/passthrough message instead of crashing on unrecognized `type` field. Go SDK parser in `internal/parser/parser.go` likely returns error on unknown types. Change to return RawMessage or fallback type so new CLI message types don't break existing consumers. Forward-compatibility. |

---

## Phase 2 - Mar 3 to Mar 16

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 9 | #619 | stop_reason on ResultMessage | Mar 3 | feat | pending | - | Python added `stop_reason: str` to ResultMessage (values: "end_turn", "max_tokens", "stop_sequence"). Add `StopReason string` field with `json:"stop_reason"` to ResultMessage in `internal/shared/message.go`. Wire through UnmarshalJSON. |
| 10 | #620 | MCP control methods + typed McpServerStatus | Mar 3 | feat | pending | - | Python added reconnect_mcp_server(name) and toggle_mcp_server(name, enabled). Also typed McpServerStatus with McpServerConnectionStatus enum ("connected"/"failed"/"needs-auth"/"pending"/"disabled") and McpServerInfo (name, version). Add ReconnectMcpServer(ctx, name) and ToggleMcpServer(ctx, name, enabled) on Client interface, new control request subtypes, wire through Transport. |
| 11 | #621 | Typed TaskStarted/TaskProgress/TaskNotification | Mar 3 | feat | pending | - | Python created typed SystemMessage subclasses: TaskStartedMessage (task_id, prompt), TaskProgressMessage (task_id, tool_name, progress), TaskNotificationMessage (task_id, status, usage). Go SDK uses generic SystemMessage with Data map. Add concrete structs for each subtype, parse on SystemMessage.Subtype in `internal/shared/message.go`. Add TaskUsage struct (total_tokens, tool_uses, duration_ms). TaskNotificationStatus enum: "completed", "failed", "stopped". |
| 12 | #622 | list_sessions / get_session_messages | Mar 3 | feat | pending | - | Python added top-level list_sessions() and get_session_messages(session_id) using CLI flags --list-sessions and --get-session-messages. Returns []SDKSessionInfo and []SessionMessage. Add package-level functions, SDKSessionInfo struct (session_id, summary, last_modified, file_size, custom_title, first_prompt, git_branch, cwd), SessionMessage struct (type, uuid, session_id, message, parent_tool_use_id). Standalone CLI invocations, not control protocol. |
| 13 | #628 | agent_id/agent_type in hook inputs | Mar 3 | feat | pending | - | Python introduced `_SubagentContextMixin` (TypedDict, total=False) and applied it to the four tool-lifecycle hook inputs the CLI populates: PreToolUseHookInput, PostToolUseHookInput, **PostToolUseFailureHookInput**, PermissionRequestHookInput. `agent_id` is the correlation key for parallel sub-agents whose hooks interleave on the same control channel; absent on the main thread. Add `AgentID *string` and `AgentType *string` (with `omitempty`) to all four hook input structs in `internal/control/types_hook.go`. NOTE: this is where the mixin deferral from item #2 (PR #535 / PostToolUseFailureHookInput) lands — revisit that struct here. |
| 14 | #630 | Fix string prompt closing stdin before MCP init | Mar 4 | fix | pending | - | Python fixed race: string prompt closed stdin before SDK MCP servers initialized. Check Go subprocess stdin in `internal/subprocess/` - verify stdin stays open until MCP init completes (wait for initialize response before closing write end). |
| 14a | #642 | Wait for graceful subprocess shutdown before SIGTERM | Mar 19 | fix | pending | - | Python added a graceful wait period before sending SIGTERM, allowing the subprocess to finish in-flight work. Check Go process lifecycle in `internal/subprocess/process.go` - verify shutdown sequence gives process time to flush before SIGTERM. |
| 15 | #648 | Typed RateLimitEvent message | Mar 12 | feat | partial | #129 | Python added dedicated RateLimitEvent message type (not SystemMessage) with RateLimitInfo: status, resets_at, rate_limit_type, utilization, overage_status, overage_resets_at, overage_disabled_reason, raw. Add RateLimitEvent struct implementing Message interface, RateLimitInfo struct, parser update to recognize type "rate_limit_event" in `internal/parser/json.go`. Also carries uuid and session_id. **Partial scope landed in Go PR #129**: RateLimitEventMessage + RateLimitInfo (status, resetsAt, rateLimitType, overageStatus, overageResetsAt, IsUsingOverage), MessageTypeRateLimitEvent + RateLimitStatusAllowed constants, parser case, public re-exports. **Deferred to complete this row**: (a) missing fields utilization, overage_disabled_reason, raw; (b) full RateLimitStatus enum (add RateLimitStatusAllowedWarning, RateLimitStatusRejected); (c) RateLimitType enum constants (five_hour, seven_day, seven_day_opus, seven_day_sonnet, overage); (d) tighten uuid/session_id to required (Python parser raises KeyError when absent); (e) remove non-Python IsUsingOverage field (Python preserves it only via raw); (f) example coverage (e.g. examples/20_debugging_and_diagnostics); (g) comment-hygiene cleanup at internal/shared/message.go:308 (issue trailer); (h) test fixes in internal/parser/json_test.go (status="blocked" is not a valid literal — use "rejected"; replace unchecked type assertions at lines 880/910 with ok-pattern). |
| 16 | #668 | rename_session | Mar 12 | feat | pending | - | Python added rename_session(session_id, new_name) to rename a session's custom title. Add RenameSession(ctx, sessionID, newName string) package-level function. Uses CLI flag or control request. |
| 17 | #670 | tag_session with Unicode sanitization | Mar 12 | feat | pending | - | Python added tag_session(session_id, tag) with Unicode sanitization (strips non-printable chars, validates length). Add TagSession(ctx, sessionID, tag string) package-level function with equivalent Unicode validation. |
| 18 | #685 | Preserve per-turn usage on AssistantMessage | Mar 16 | feat | pending | - | Python added `usage: dict` to AssistantMessage for per-turn token stats (input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens). Add `Usage map[string]any` with `json:"usage,omitempty"` to AssistantMessage in `internal/shared/message.go`. |
| 19 | #684 | skills/memory/mcpServers on AgentDefinition | Mar 16 | feat | pending | - | Python added skills (list), memory (config), mcpServers (map) to AgentDefinition. Add `Skills []any`, `Memory any`, `McpServers map[string]McpServerConfig` with JSON tags to AgentDefinition in `internal/shared/options.go`. |
| 20 | #686 | default-if-absent for CLAUDE_CODE_ENTRYPOINT | Mar 16 | fix | pending | - | Python only sets CLAUDE_CODE_ENTRYPOINT if not already in env (matching TS SDK). Check `internal/subprocess/config.go` - change from unconditional set to set-if-absent so users can override. |

---

## Phase 3 - Mar 20 to Mar 30

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 21 | #667 | tag/created_at on SDKSessionInfo + get_session_info | Mar 20 | feat | pending | - | Python added tag and created_at fields to SDKSessionInfo. Added get_session_info(session_id) function. Add `Tag *string`, `CreatedAt *string` to SDKSessionInfo. Add GetSessionInfo(ctx, sessionID) package-level function. |
| 22 | #591 | SystemPromptFile support | Mar 25 | feat | pending | - | Python added SystemPromptFile TypedDict passing --system-prompt-file to CLI. Add SystemPromptFile struct (Path string), update system_prompt option handling, add --system-prompt-file flag in `internal/cli/command.go`. |
| 23 | #717 | propagate is_error from SDK MCP tool results | Mar 25 | fix | pending | - | Python fixed is_error flag propagation from McpToolResult to CLI. Check `internal/control/mcp.go` - verify IsError on McpToolResult is included in JSON response to CLI. |
| 24 | #718 | Preserve dropped fields on AssistantMessage/ResultMessage | Mar 25 | feat | pending | - | Python captures unknown JSON fields into `extra_fields: dict` catch-all to prevent data loss when CLI adds new fields. Add `ExtraFields map[string]any` to AssistantMessage and ResultMessage, populated during UnmarshalJSON by collecting keys not in known fields. Forward-compatibility pattern. |
| 25 | #719 | add missing dontAsk permission mode | Mar 25 | fix | pending | - | Python added "dontAsk" to PermissionMode (was missing despite being valid CLI mode). Add `PermissionModeDontAsk PermissionMode = "dontAsk"`. Check if Go SDK already has this. |
| 26 | #720 | remove duplicate version warning | Mar 25 | fix | pending | - | Python removed duplicate CLI version warning. Check `internal/cli/version.go` - verify warning fires only once per session. |
| 27 | #723 | skip non-JSON lines on CLI stdout | Mar 25 | fix | pending | - | Python skips non-JSON debug lines on stdout (DEBUG env in Docker/Lambda). Check `internal/parser/parser.go` - verify graceful skip of lines not starting with `{`. May already be handled by speculative parsing. |
| 28 | #725 | handle resource_link/embedded resource in SDK MCP | Mar 25 | fix | pending | - | Python added resource_link and embedded_resource content types in SDK MCP tool responses. Check `mcp.go` - add support for these content types in McpContent or pass through as-is. |
| 29 | #731 | remove stdin timeout for hooks/SDK MCP | Mar 25 | fix | pending | - | Python removed write timeout on stdin for hook/MCP responses (can take long). Check `internal/subprocess/io.go` - verify no artificial timeout on control response writes. |
| 30 | #729 | SIGKILL fallback when SIGTERM blocks | Mar 26 | fix | pending | - | Python added SIGKILL escalation when SIGTERM handler blocks. Go SDK has SIGTERM->wait->SIGKILL in `internal/subprocess/process.go` - verify wait timeout (5s) and SIGKILL fires if process doesn't exit. |
| 31 | #732 | filter CLAUDECODE env var from subprocess | Mar 26 | fix | pending | - | Python filters CLAUDECODE env var from subprocess to prevent child CLI inheritance. Check `internal/subprocess/config.go` - add CLAUDECODE to filtered env vars. |
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
| 42 | #778 | omit --setting-sources when empty | Mar 30 | fix | pending | - | Python omits --setting-sources when empty instead of passing empty value. Check `internal/cli/command.go` - verify empty SettingSources doesn't produce `--setting-sources ""`. |
| 43 | #780 | background task for string prompts with hooks/MCP | Mar 30 | fix | pending | - | Python fixed deadlock with string prompt + hooks/MCP by spawning stdin write as background task. Verify Go SDK has no deadlock when hooks/MCP configured with string prompt. |

---

## Phase 4 - Mar 31 to Apr 8

| # | Py PR | Title | Merged | Cat | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----------|:------|:------|
| 44 | #782 | background/effort/permissionMode on AgentDefinition | Mar 31 | feat | pending | - | Python added background (bool), effort (string), permissionMode (PermissionMode) to AgentDefinition. Add `Background *bool`, `Effort *string`, `PermissionMode *PermissionMode` with JSON tags. |
| 45 | #756 | forward maxResultSizeChars via _meta | Apr 2 | fix | pending | - | Python forwards maxResultSizeChars in _meta of MCP tool results for large results (>50K chars). Add _meta.maxResultSizeChars to tool result JSON when exceeding threshold. |
| 46 | #785 | 'auto' PermissionMode | Apr 7 | feat | pending | - | Python added "auto" PermissionMode (auto-approves safe tools, prompts for dangerous). Add `PermissionModeAuto PermissionMode = "auto"` in `internal/shared/options.go`. |
| 47 | #796 | --thinking flag for adaptive/disabled | Apr 7 | fix | pending | - | Python fixed CLI flags: adaptive -> `--thinking adaptive`, disabled -> `--thinking disabled` (not budget tokens). Update CLI builder in `internal/cli/command.go` to handle ThinkingConfig types correctly. |
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
