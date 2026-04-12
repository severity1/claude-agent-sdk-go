# Python SDK PR Replay Tracker

Tracks all Python SDK PRs that need to be replayed into the Go SDK to restore parity. PRs are listed in merge-order - replay them top to bottom.

## Snapshot

| Field | Value |
|:------|:------|
| Snapshot date | April 12, 2026 |
| Coverage window | Jan 6, 2026 - April 12, 2026 |
| Parity milestone | Go PR #77 (Jan 6, 2026) - formal parity documentation |
| Last ported feature | Go PR #99 (Jan 24, 2026) - tool_use_result (Python PR #495) |
| Python SDK at checkpoint | v0.1.22 |
| Python SDK at snapshot | v0.1.58 |

PRs merged after April 12, 2026 are NOT tracked here. Add them manually and update the snapshot date.

## Summary

| Priority | Count | Description |
|:---------|:------|:------------|
| P0 | 8 | Message types/parsing - broken deserialization if missing |
| P1 | 25 | New client methods, options, control protocol features |
| P2 | 16 | Bug fixes - evaluate Go applicability |
| Skip | 31 | CI/CD, Python typing, async-specific, PyPI/wheel, CLI bumps |
| **Actionable** | **49** | |

## Priority Scale

- **P0 (Critical):** New fields or types on messages. Missing these means the Go SDK silently drops data from CLI responses or fails to deserialize.
- **P1 (High):** New client methods, configuration options, or control protocol features. The SDK works without these but lacks functionality the Python SDK offers.
- **P2 (Medium):** Bug fixes from the Python SDK. Each must be evaluated against the Go codebase - some may already be handled differently, some may need porting.
- **Skip:** Not applicable to Go SDK. CI/CD workflows, Python-specific typing (typing_extensions, TypedDict compat, Annotated), Python async-specific fixes (cancel scope, event loop), PyPI/wheel publishing, CLI version bumps, docs-only changes.

## Implementation Waves (Reference)

Waves group related PRs by area. Use as a reference when working on a PR to see what else touches the same files.

| Wave | Area | PRs | Go Files |
|:-----|:-----|:----|:---------|
| 1 | Type Safety (P0) | #506, #598, #619, #621, #648, #685, #718, #749 | `internal/shared/message.go`, `internal/parser/parser.go` |
| 2 | Hook System | #535, #545, #628 | `internal/control/types_hook.go`, `internal/control/hooks.go` |
| 3 | Session Management | #622, #667, #668, #670, #744, #750 | `client.go`, `options.go`, `internal/control/` |
| 4 | Agent/Options | #468, #565, #591, #684, #747, #759, #782, #785, #796, #797 | `internal/shared/options.go`, `options.go`, `internal/cli/command.go` |
| 5 | MCP Enhancements | #516, #551, #620 | `mcp.go`, `client.go`, `internal/control/` |
| 6 | New Client Methods | #751, #754, #764 | `client.go`, `internal/control/types.go`, `transport.go` |
| 7 | Bug Fix Audit | #630, #686, #717, #719, #720, #723, #725, #729, #731, #732, #743, #756, #769, #778, #780 | various |

---

## Replay Queue (Chronological)

Replay in order. Each row is one Python SDK PR, sorted by merge date.

| # | Py PR | Title | Merged | Cat | Pri | Wave | Go Status | Go PR | Notes |
|:--|:------|:------|:-------|:----|:----|:-----|:----------|:------|:------|
| - | #495 | tool_use_result on UserMessage | Jan 23 | feat | - | - | done | #99 | Added ToolUseResult map[string]any field to UserMessage with HasToolUseResult()/GetToolUseResult() helpers |
| 1 | #516 | get_mcp_status() method | Jan 26 | feat | P1 | 5 | pending | - | Python added get_mcp_status() returning McpStatusResponse with list of McpServerStatus (name, status, serverInfo, error, config, scope, tools). Add GetMcpStatus(ctx) to Client interface, control request subtype "get_mcp_status", McpServerStatus/McpStatusResponse types in `internal/control/types.go`, wire through Transport. |
| 2 | #535 | PostToolUseFailure hook event | Jan 30 | feat | P1 | 2 | pending | - | Python added "PostToolUseFailure" hook event firing when tool execution fails (distinct from PostToolUse on success). Add `HookEventPostToolUseFailure = "PostToolUseFailure"` constant, PostToolUseFailureHookInput (embeds BaseHookInput, has ToolName, ToolInput, Error), PostToolUseFailureHookSpecificOutput in `internal/control/types_hook.go`. |
| 3 | #506 | Properly populate AssistantMessage error field | Feb 3 | fix | P0 | 1 | pending | - | Python fixed AssistantMessage.error not being populated from JSON during deserialization. Go SDK has AssistantMessage.Error *AssistantMessageError - verify UnmarshalJSON in `internal/shared/message.go` correctly parses the `error` object with type/message fields. Test with rate_limit, billing_error, server_error payloads. |
| 4 | #545 | Missing hook events + fix fields | Feb 3 | feat | P1 | 2 | pending | - | Python added 3 hook events: "Notification" (system notifications), "SubagentStart" (subagent spawned), "PermissionRequest" (permission requested). Each has HookInput type. Also fixed existing hook fields. Add 3 HookEvent constants, 3 HookInput structs, 3 HookSpecificOutput structs in `internal/control/types_hook.go`. Add SubagentContextMixin equivalent (AgentID, AgentType *string) to relevant inputs. |
| 5 | #551 | MCP tool annotations | Feb 5 | feat | P1 | 5 | pending | - | Python added McpToolAnnotations (readOnly, destructive, openWorld bools) and McpToolInfo (name, description, annotations). Returned in MCP server status and tool listings. Add structs in `mcp.go` or `internal/shared/options.go`. Wire into McpServerStatus.Tools. |
| 6 | #468 | Send agent definitions via initialize request | Feb 5 | feat | P1 | 4 | pending | - | Python sends agents in initialize control request instead of --agents CLI flag. Add `Agents map[string]AgentDefinition` to InitializeRequest in `internal/control/types.go`. Update `internal/control/protocol.go` to include agents. Remove/deprecate --agents flag in `internal/cli/`. |
| 7 | #565 | ThinkingConfig types and effort option | Feb 11 | feat | P1 | 4 | pending | - | Python replaced max_thinking_tokens with ThinkingConfig union: ThinkingConfigAdaptive (type="adaptive"), ThinkingConfigEnabled (type="enabled", budget_tokens), ThinkingConfigDisabled (type="disabled"). Added effort option ("low"/"medium"/"high"/"max"). Add ThinkingConfig interface + 3 structs, WithThinking(), WithEffort() in `options.go`. Update CLI flags: adaptive -> `--thinking adaptive`, disabled -> `--thinking disabled`, enabled -> `--thinking-budget N`. Deprecate WithMaxThinkingTokens(). |
| 8 | #598 | Handle unknown message types gracefully | Feb 20 | fix | P0 | 1 | pending | - | Python returns a raw/passthrough message instead of crashing on unrecognized `type` field. Go SDK parser in `internal/parser/parser.go` likely returns error on unknown types. Change to return RawMessage or fallback type so new CLI message types don't break existing consumers. Forward-compatibility. |
| 9 | #619 | stop_reason on ResultMessage | Mar 3 | feat | P0 | 1 | pending | - | Python added `stop_reason: str` to ResultMessage (values: "end_turn", "max_tokens", "stop_sequence"). Add `StopReason string` field with `json:"stop_reason"` to ResultMessage in `internal/shared/message.go`. Wire through UnmarshalJSON. |
| 10 | #620 | MCP control methods + typed McpServerStatus | Mar 3 | feat | P1 | 5 | pending | - | Python added reconnect_mcp_server(name) and toggle_mcp_server(name, enabled). Also typed McpServerStatus with McpServerConnectionStatus enum ("connected"/"failed"/"needs-auth"/"pending"/"disabled") and McpServerInfo (name, version). Add ReconnectMcpServer(ctx, name) and ToggleMcpServer(ctx, name, enabled) on Client interface, new control request subtypes, wire through Transport. |
| 11 | #621 | Typed TaskStarted/TaskProgress/TaskNotification | Mar 3 | feat | P0 | 1 | pending | - | Python created typed SystemMessage subclasses: TaskStartedMessage (task_id, prompt), TaskProgressMessage (task_id, tool_name, progress), TaskNotificationMessage (task_id, status, usage). Go SDK uses generic SystemMessage with Data map. Add concrete structs for each subtype, parse on SystemMessage.Subtype in `internal/shared/message.go`. Add TaskUsage struct (total_tokens, tool_uses, duration_ms). TaskNotificationStatus enum: "completed", "failed", "stopped". |
| 12 | #622 | list_sessions / get_session_messages | Mar 3 | feat | P1 | 3 | pending | - | Python added top-level list_sessions() and get_session_messages(session_id) using CLI flags --list-sessions and --get-session-messages. Returns []SDKSessionInfo and []SessionMessage. Add package-level functions, SDKSessionInfo struct (session_id, summary, last_modified, file_size, custom_title, first_prompt, git_branch, cwd), SessionMessage struct (type, uuid, session_id, message, parent_tool_use_id). Standalone CLI invocations, not control protocol. |
| 13 | #628 | agent_id/agent_type in hook inputs | Mar 3 | feat | P1 | 2 | pending | - | Python added optional agent_id and agent_type to PreToolUseHookInput and PostToolUseHookInput via _SubagentContextMixin. Identifies which agent triggered tool use. Add `AgentID *string` and `AgentType *string` to both hook input structs in `internal/control/types_hook.go`. |
| 14 | #630 | Fix string prompt closing stdin before MCP init | Mar 4 | fix | P2 | 7 | pending | - | Python fixed race: string prompt closed stdin before SDK MCP servers initialized. Check Go subprocess stdin in `internal/subprocess/` - verify stdin stays open until MCP init completes (wait for initialize response before closing write end). |
| 14a | #642 | Wait for graceful subprocess shutdown before SIGTERM | Mar 19 | fix | P2 | 7 | pending | - | Python added a graceful wait period before sending SIGTERM, allowing the subprocess to finish in-flight work. Check Go process lifecycle in `internal/subprocess/process.go` - verify shutdown sequence gives process time to flush before SIGTERM. |
| 15 | #648 | Typed RateLimitEvent message | Mar 12 | feat | P0 | 1 | pending | - | Python added dedicated RateLimitEvent message type (not SystemMessage) with RateLimitInfo: status, resets_at, rate_limit_type, utilization, overage_status, overage_resets_at, overage_disabled_reason, raw. Add RateLimitEvent struct implementing Message interface, RateLimitInfo struct, parser update to recognize type "rate_limit" in `internal/parser/parser.go`. Also carries uuid and session_id. |
| 16 | #668 | rename_session | Mar 12 | feat | P1 | 3 | pending | - | Python added rename_session(session_id, new_name) to rename a session's custom title. Add RenameSession(ctx, sessionID, newName string) package-level function. Uses CLI flag or control request. |
| 17 | #670 | tag_session with Unicode sanitization | Mar 12 | feat | P1 | 3 | pending | - | Python added tag_session(session_id, tag) with Unicode sanitization (strips non-printable chars, validates length). Add TagSession(ctx, sessionID, tag string) package-level function with equivalent Unicode validation. |
| 18 | #685 | Preserve per-turn usage on AssistantMessage | Mar 16 | feat | P0 | 1 | pending | - | Python added `usage: dict` to AssistantMessage for per-turn token stats (input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens). Add `Usage map[string]any` with `json:"usage,omitempty"` to AssistantMessage in `internal/shared/message.go`. |
| 19 | #684 | skills/memory/mcpServers on AgentDefinition | Mar 16 | feat | P1 | 4 | pending | - | Python added skills (list), memory (config), mcpServers (map) to AgentDefinition. Add `Skills []any`, `Memory any`, `McpServers map[string]McpServerConfig` with JSON tags to AgentDefinition in `internal/shared/options.go`. |
| 20 | #686 | default-if-absent for CLAUDE_CODE_ENTRYPOINT | Mar 16 | fix | P2 | 7 | pending | - | Python only sets CLAUDE_CODE_ENTRYPOINT if not already in env (matching TS SDK). Check `internal/subprocess/config.go` - change from unconditional set to set-if-absent so users can override. |
| 21 | #667 | tag/created_at on SDKSessionInfo + get_session_info | Mar 20 | feat | P1 | 3 | pending | - | Python added tag and created_at fields to SDKSessionInfo. Added get_session_info(session_id) function. Add `Tag *string`, `CreatedAt *string` to SDKSessionInfo. Add GetSessionInfo(ctx, sessionID) package-level function. |
| 22 | #591 | SystemPromptFile support | Mar 25 | feat | P1 | 4 | pending | - | Python added SystemPromptFile TypedDict passing --system-prompt-file to CLI. Add SystemPromptFile struct (Path string), update system_prompt option handling, add --system-prompt-file flag in `internal/cli/command.go`. |
| 23 | #717 | propagate is_error from SDK MCP tool results | Mar 25 | fix | P2 | 7 | pending | - | Python fixed is_error flag propagation from McpToolResult to CLI. Check `internal/control/mcp.go` - verify IsError on McpToolResult is included in JSON response to CLI. |
| 24 | #718 | Preserve dropped fields on AssistantMessage/ResultMessage | Mar 25 | feat | P0 | 1 | pending | - | Python captures unknown JSON fields into `extra_fields: dict` catch-all to prevent data loss when CLI adds new fields. Add `ExtraFields map[string]any` to AssistantMessage and ResultMessage, populated during UnmarshalJSON by collecting keys not in known fields. Forward-compatibility pattern. |
| 25 | #719 | add missing dontAsk permission mode | Mar 25 | fix | P2 | 7 | pending | - | Python added "dontAsk" to PermissionMode (was missing despite being valid CLI mode). Add `PermissionModeDontAsk PermissionMode = "dontAsk"`. Check if Go SDK already has this. |
| 26 | #720 | remove duplicate version warning | Mar 25 | fix | P2 | 7 | pending | - | Python removed duplicate CLI version warning. Check `internal/cli/version.go` - verify warning fires only once per session. |
| 27 | #723 | skip non-JSON lines on CLI stdout | Mar 25 | fix | P2 | 7 | pending | - | Python skips non-JSON debug lines on stdout (DEBUG env in Docker/Lambda). Check `internal/parser/parser.go` - verify graceful skip of lines not starting with `{`. May already be handled by speculative parsing. |
| 28 | #725 | handle resource_link/embedded resource in SDK MCP | Mar 25 | fix | P2 | 7 | pending | - | Python added resource_link and embedded_resource content types in SDK MCP tool responses. Check `mcp.go` - add support for these content types in McpContent or pass through as-is. |
| 29 | #731 | remove stdin timeout for hooks/SDK MCP | Mar 25 | fix | P2 | 7 | pending | - | Python removed write timeout on stdin for hook/MCP responses (can take long). Check `internal/subprocess/io.go` - verify no artificial timeout on control response writes. |
| 30 | #729 | SIGKILL fallback when SIGTERM blocks | Mar 26 | fix | P2 | 7 | pending | - | Python added SIGKILL escalation when SIGTERM handler blocks. Go SDK has SIGTERM->wait->SIGKILL in `internal/subprocess/process.go` - verify wait timeout (5s) and SIGKILL fires if process doesn't exit. |
| 31 | #732 | filter CLAUDECODE env var from subprocess | Mar 26 | fix | P2 | 7 | pending | - | Python filters CLAUDECODE env var from subprocess to prevent child CLI inheritance. Check `internal/subprocess/config.go` - add CLAUDECODE to filtered env vars. |
| 32 | #744 | fork_session, delete_session, offset pagination | Mar 26 | feat | P1 | 3 | pending | - | Python added fork_session(session_id) -> ForkSessionResult (new_session_id), delete_session(session_id), offset/limit params on list_sessions(). Add ForkSession(), DeleteSession() functions, ForkSessionResult struct, update ListSessions with pagination. |
| 33 | #747 | task_budget option | Mar 26 | feat | P1 | 4 | pending | - | Python added TaskBudget TypedDict (total int) and task_budget option. Passed as --task-budget CLI flag. Add TaskBudget struct, WithTaskBudget() in `options.go`, CLI flag in `internal/cli/`. |
| 34 | #743 | pass initialize_timeout from env var | Mar 26 | fix | P2 | 7 | pending | - | Python reads CLAUDE_CODE_STREAM_CLOSE_TIMEOUT env var to override initialize timeout. Check if Go SDK respects this, add support if not. |
| 35 | #749 | errors field on ResultMessage | Mar 27 | fix | P0 | 1 | pending | - | Python added `errors: list[str]` to ResultMessage for non-fatal session errors. Add `Errors []string` with `json:"errors,omitempty"` to ResultMessage. Distinct from IsError (fatal failure flag). |
| 36 | #759 | disallowedTools/maxTurns/initialPrompt on AgentDefinition | Mar 27 | feat | P1 | 4 | pending | - | Python added disallowedTools ([]string), maxTurns (int), initialPrompt (string) to AgentDefinition. Add `DisallowedTools []string`, `MaxTurns *int`, `InitialPrompt *string` with JSON tags to AgentDefinition. |
| 37 | #750 | session_id on ClaudeAgentOptions | Mar 28 | feat | P1 | 3 | pending | - | Python added session_id option for custom session IDs. Passed as --session-id CLI flag. Add WithSessionID(id string) option, `SessionID *string` to Options, CLI flag in `internal/cli/`. |
| 38 | #754 | tool_use_id/agent_id in ToolPermissionContext | Mar 28 | feat | P1 | 6 | pending | - | Python added tool_use_id and agent_id to ToolPermissionContext for richer permission callback context. Add `ToolUseID string` and `AgentID string` to ToolPermissionContext in `internal/control/types.go`. |
| 39 | #764 | get_context_usage() | Mar 28 | feat | P1 | 6 | pending | - | Python added get_context_usage() returning ContextUsageResponse: categories ([]ContextUsageCategory with name/tokens/color/isDeferred), totalTokens, maxTokens, percentage, model, optional autoCompactThreshold/mcpTools/agents. Add GetContextUsage(ctx) to Client, ContextUsageResponse/ContextUsageCategory structs, control request subtype, wire through Transport. |
| 40 | #751 | control_cancel_request handling | Mar 28 | feat | P1 | 6 | pending | - | Python handles control_cancel_request from CLI to cancel pending control requests (e.g., timed-out permission callbacks). Update `internal/control/protocol.go` to handle incoming "cancel_request" by canceling context of pending request by request_id. |
| 41 | #769 | send string prompt in connect() | Mar 28 | fix | P2 | 7 | pending | - | Python fixed connect(prompt="...") silently dropping prompt. Verify Go Client.Connect() sends prompt via stdin after connection established. |
| 42 | #778 | omit --setting-sources when empty | Mar 30 | fix | P2 | 7 | pending | - | Python omits --setting-sources when empty instead of passing empty value. Check `internal/cli/command.go` - verify empty SettingSources doesn't produce `--setting-sources ""`. |
| 43 | #780 | background task for string prompts with hooks/MCP | Mar 30 | fix | P2 | 7 | pending | - | Python fixed deadlock with string prompt + hooks/MCP by spawning stdin write as background task. Verify Go SDK has no deadlock when hooks/MCP configured with string prompt. |
| 44 | #782 | background/effort/permissionMode on AgentDefinition | Mar 31 | feat | P1 | 4 | pending | - | Python added background (bool), effort (string), permissionMode (PermissionMode) to AgentDefinition. Add `Background *bool`, `Effort *string`, `PermissionMode *PermissionMode` with JSON tags. |
| 45 | #756 | forward maxResultSizeChars via _meta | Apr 2 | fix | P2 | 7 | pending | - | Python forwards maxResultSizeChars in _meta of MCP tool results for large results (>50K chars). Add _meta.maxResultSizeChars to tool result JSON when exceeding threshold. |
| 46 | #785 | 'auto' PermissionMode | Apr 7 | feat | P1 | 4 | pending | - | Python added "auto" PermissionMode (auto-approves safe tools, prompts for dangerous). Add `PermissionModeAuto PermissionMode = "auto"` in `internal/shared/options.go`. |
| 47 | #796 | --thinking flag for adaptive/disabled | Apr 7 | fix | P1 | 4 | pending | - | Python fixed CLI flags: adaptive -> `--thinking adaptive`, disabled -> `--thinking disabled` (not budget tokens). Update CLI builder in `internal/cli/command.go` to handle ThinkingConfig types correctly. |
| 48 | #797 | exclude_dynamic_sections on SystemPromptPreset | Apr 8 | feat | P1 | 4 | pending | - | Python added exclude_dynamic_sections bool to SystemPromptPreset for cross-user prompt caching. Add ExcludeDynamicSections bool to SystemPromptPreset struct (create if needed), pass as CLI flag. |

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
| #762 | Annotated for per-parameter descriptions in @tool | Mar 28 | Python typing.Annotated feature |

---

## How to Update

1. **Starting work:** Set Go Status to `in-progress` for the current row
2. **PR merged:** Set Go Status to `done`, fill Go PR column with `#N`
3. **Not applicable:** Set Go Status to `skip`, add reason in Notes
4. **New Python PRs (after April 12, 2026):** Append rows to the replay queue in merge order, update snapshot date
5. **Keep summary stats in sync** with the table counts
6. **Re-evaluate priorities** as replay progresses - some P2s may become P0 based on user reports
