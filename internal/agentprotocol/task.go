package agentprotocol

type ContextItem struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Content string `json:"content"`
}

type Start struct {
	TaskID         string        `json:"task_id"`
	AgentID        int64         `json:"agent_id"`
	ConversationID int64         `json:"conversation_id"`
	ThreadRootID   int64         `json:"thread_root_id"`
	Prompt         string        `json:"prompt"`
	Context        []ContextItem `json:"context"`
}

type Steer struct {
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

type Interrupt struct {
	TaskID string `json:"task_id"`
}

type Resume struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

type Progress struct {
	TaskID  string `json:"task_id"`
	Phase   string `json:"phase"`
	Summary string `json:"summary"`
	Percent int    `json:"percent"`
}

type Failure struct {
	TaskID string `json:"task_id"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type Usage struct {
	TaskID       string `json:"task_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}
