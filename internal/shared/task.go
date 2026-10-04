package shared

import "fmt"

// System message subtypes that report a task's lifecycle. A task is work the
// CLI tracks by task ID, such as a subagent started by the Agent/Task tool or
// a background Bash command.
const (
	SystemSubtypeTaskStarted      = "task_started"
	SystemSubtypeTaskProgress     = "task_progress"
	SystemSubtypeTaskNotification = "task_notification"
	SystemSubtypeTaskUpdated      = "task_updated"
)

// TaskNotificationStatus is the status of a task_notification message.
type TaskNotificationStatus string

// TaskNotificationStatus values.
const (
	TaskNotificationStatusCompleted TaskNotificationStatus = "completed"
	TaskNotificationStatusFailed    TaskNotificationStatus = "failed"
	TaskNotificationStatusStopped   TaskNotificationStatus = "stopped"
)

// TaskUpdatedStatus is the status reported inside a task_updated patch.
// Pending, running and paused are non-terminal; completed, failed and killed
// are terminal. A task_updated patch reports a stopped task as killed, while
// task_notification reports it as stopped.
type TaskUpdatedStatus string

// TaskUpdatedStatus values.
const (
	TaskUpdatedStatusPending   TaskUpdatedStatus = "pending"
	TaskUpdatedStatusRunning   TaskUpdatedStatus = "running"
	TaskUpdatedStatusPaused    TaskUpdatedStatus = "paused"
	TaskUpdatedStatusCompleted TaskUpdatedStatus = "completed"
	TaskUpdatedStatusFailed    TaskUpdatedStatus = "failed"
	TaskUpdatedStatusKilled    TaskUpdatedStatus = "killed"
)

// IsTerminalTaskStatus reports whether status means the task has finished:
// completed, failed, stopped or killed. It accepts the status of both a
// TaskNotificationMessage and a TaskUpdatedMessage, so a host can clear a task
// on a terminal status from either message.
func IsTerminalTaskStatus(status string) bool {
	switch status {
	case string(TaskNotificationStatusCompleted), string(TaskNotificationStatusFailed),
		string(TaskNotificationStatusStopped), string(TaskUpdatedStatusKilled):
		return true
	}
	return false
}

// TaskUsage is the usage reported in task_progress and task_notification messages.
type TaskUsage struct {
	TotalTokens int `json:"total_tokens"`
	ToolUses    int `json:"tool_uses"`
	DurationMs  int `json:"duration_ms"`
}

// TaskStartedMessage is the typed form of a task_started system message.
// The embedded SystemMessage keeps the subtype and the raw payload, and
// json.Marshal writes that raw payload. The parser always returns
// *SystemMessage, so get this type with an AsTask* method, not a type switch.
type TaskStartedMessage struct {
	SystemMessage
	TaskID      string
	Description string
	UUID        string
	SessionID   string
	ToolUseID   *string
	TaskType    *string
}

// TaskProgressMessage is the typed form of a task_progress system message.
// The embedded SystemMessage keeps the subtype and the raw payload, and
// json.Marshal writes that raw payload. The parser always returns
// *SystemMessage, so get this type with an AsTask* method, not a type switch.
type TaskProgressMessage struct {
	SystemMessage
	TaskID       string
	Description  string
	Usage        TaskUsage
	UUID         string
	SessionID    string
	ToolUseID    *string
	LastToolName *string
}

// TaskNotificationMessage is the typed form of a task_notification system
// message, sent when a task completes, fails or is stopped. Not every finished
// task sends one: a background task can report its end only as a
// TaskUpdatedMessage with a terminal status (see IsTerminalTaskStatus).
// The embedded SystemMessage keeps the subtype and the raw payload, and
// json.Marshal writes that raw payload. The parser always returns
// *SystemMessage, so get this type with an AsTask* method, not a type switch.
type TaskNotificationMessage struct {
	SystemMessage
	TaskID     string
	Status     TaskNotificationStatus
	OutputFile string
	Summary    string
	UUID       string
	SessionID  string
	ToolUseID  *string
	Usage      *TaskUsage
}

// TaskUpdatedMessage is the typed form of a task_updated system message.
// Patch holds the task fields that changed; Status is Patch["status"] when
// that is a string. A task stopped with StopTask can report its end only
// here, with status killed and no TaskNotificationMessage.
// The embedded SystemMessage keeps the subtype and the raw payload, and
// json.Marshal writes that raw payload. The parser always returns
// *SystemMessage, so get this type with an AsTask* method, not a type switch.
type TaskUpdatedMessage struct {
	SystemMessage
	TaskID    string
	Patch     map[string]any
	Status    *TaskUpdatedStatus
	SessionID *string
	UUID      *string
}

// AsTaskStarted returns the typed form of a task_started message. ok is false
// for any other subtype or when a required field is missing.
func (m *SystemMessage) AsTaskStarted() (msg *TaskStartedMessage, ok bool) {
	t, err := decodeTaskStarted(m)
	return t, err == nil
}

// AsTaskProgress returns the typed form of a task_progress message. ok is
// false for any other subtype or when a required field is missing.
func (m *SystemMessage) AsTaskProgress() (msg *TaskProgressMessage, ok bool) {
	t, err := decodeTaskProgress(m)
	return t, err == nil
}

// AsTaskNotification returns the typed form of a task_notification message.
// ok is false for any other subtype or when a required field is missing.
func (m *SystemMessage) AsTaskNotification() (msg *TaskNotificationMessage, ok bool) {
	t, err := decodeTaskNotification(m)
	return t, err == nil
}

// AsTaskUpdated returns the typed form of a task_updated message. ok is false
// only for any other subtype: every field of a task_updated message is read
// leniently.
func (m *SystemMessage) AsTaskUpdated() (msg *TaskUpdatedMessage, ok bool) {
	if m == nil || m.Subtype != SystemSubtypeTaskUpdated {
		return nil, false
	}
	return decodeTaskUpdated(m), true
}

// ValidateTaskMessage returns a MessageParseError when a task_started,
// task_progress or task_notification message lacks a required field. It
// returns nil for every other message, including any task_updated message.
func ValidateTaskMessage(m *SystemMessage) error {
	var err error
	switch m.Subtype {
	case SystemSubtypeTaskStarted:
		_, err = decodeTaskStarted(m)
	case SystemSubtypeTaskProgress:
		_, err = decodeTaskProgress(m)
	case SystemSubtypeTaskNotification:
		_, err = decodeTaskNotification(m)
	}
	return err
}

// taskFields reads fields from a task message's payload and keeps the first
// missing required field as a MessageParseError.
type taskFields struct {
	subtype string
	data    map[string]any
	err     error
}

func newTaskFields(m *SystemMessage, subtype string) (*taskFields, error) {
	if m == nil || m.Subtype != subtype {
		return nil, fmt.Errorf("not a %s message", subtype)
	}
	return &taskFields{subtype: subtype, data: m.Data}, nil
}

func (f *taskFields) missing(key string) {
	if f.err == nil {
		f.err = NewMessageParseError(fmt.Sprintf("%s message missing %s field", f.subtype, key), f.data)
	}
}

func (f *taskFields) str(key string) string {
	s, ok := f.data[key].(string)
	if !ok {
		f.missing(key)
	}
	return s
}

func (f *taskFields) optStr(key string) *string {
	if s, ok := f.data[key].(string); ok {
		return &s
	}
	return nil
}

func (f *taskFields) usage() TaskUsage {
	u, ok := f.data["usage"].(map[string]any)
	if !ok {
		f.missing("usage")
	}
	return taskUsageFrom(u)
}

func (f *taskFields) optUsage() *TaskUsage {
	u, ok := f.data["usage"].(map[string]any)
	if !ok {
		return nil
	}
	usage := taskUsageFrom(u)
	return &usage
}

func taskUsageFrom(u map[string]any) TaskUsage {
	return TaskUsage{
		TotalTokens: intFrom(u["total_tokens"]),
		ToolUses:    intFrom(u["tool_uses"]),
		DurationMs:  intFrom(u["duration_ms"]),
	}
}

func intFrom(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func decodeTaskStarted(m *SystemMessage) (*TaskStartedMessage, error) {
	f, err := newTaskFields(m, SystemSubtypeTaskStarted)
	if err != nil {
		return nil, err
	}
	msg := &TaskStartedMessage{
		SystemMessage: *m,
		TaskID:        f.str("task_id"),
		Description:   f.str("description"),
		UUID:          f.str("uuid"),
		SessionID:     f.str("session_id"),
		ToolUseID:     f.optStr("tool_use_id"),
		TaskType:      f.optStr("task_type"),
	}
	if f.err != nil {
		return nil, f.err
	}
	return msg, nil
}

func decodeTaskProgress(m *SystemMessage) (*TaskProgressMessage, error) {
	f, err := newTaskFields(m, SystemSubtypeTaskProgress)
	if err != nil {
		return nil, err
	}
	msg := &TaskProgressMessage{
		SystemMessage: *m,
		TaskID:        f.str("task_id"),
		Description:   f.str("description"),
		Usage:         f.usage(),
		UUID:          f.str("uuid"),
		SessionID:     f.str("session_id"),
		ToolUseID:     f.optStr("tool_use_id"),
		LastToolName:  f.optStr("last_tool_name"),
	}
	if f.err != nil {
		return nil, f.err
	}
	return msg, nil
}

func decodeTaskNotification(m *SystemMessage) (*TaskNotificationMessage, error) {
	f, err := newTaskFields(m, SystemSubtypeTaskNotification)
	if err != nil {
		return nil, err
	}
	msg := &TaskNotificationMessage{
		SystemMessage: *m,
		TaskID:        f.str("task_id"),
		Status:        TaskNotificationStatus(f.str("status")),
		OutputFile:    f.str("output_file"),
		Summary:       f.str("summary"),
		UUID:          f.str("uuid"),
		SessionID:     f.str("session_id"),
		ToolUseID:     f.optStr("tool_use_id"),
		Usage:         f.optUsage(),
	}
	if f.err != nil {
		return nil, f.err
	}
	return msg, nil
}

func decodeTaskUpdated(m *SystemMessage) *TaskUpdatedMessage {
	f := &taskFields{subtype: SystemSubtypeTaskUpdated, data: m.Data}
	patch, ok := m.Data["patch"].(map[string]any)
	if !ok {
		patch = map[string]any{}
	}
	taskID, _ := m.Data["task_id"].(string)
	msg := &TaskUpdatedMessage{
		SystemMessage: *m,
		TaskID:        taskID,
		Patch:         patch,
		SessionID:     f.optStr("session_id"),
		UUID:          f.optStr("uuid"),
	}
	if s, ok := patch["status"].(string); ok {
		status := TaskUpdatedStatus(s)
		msg.Status = &status
	}
	return msg
}
