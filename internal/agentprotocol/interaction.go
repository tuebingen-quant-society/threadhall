package agentprotocol

type AnswerValue struct {
	QuestionID string   `json:"question_id"`
	Value      []string `json:"value"`
}

type Answer struct {
	TaskID        string        `json:"task_id"`
	InteractionID string        `json:"interaction_id"`
	Answers       []AnswerValue `json:"answers"`
}

type Approval struct {
	TaskID        string `json:"task_id"`
	InteractionID string `json:"interaction_id"`
	Action        string `json:"action"`
	Summary       string `json:"summary"`
	Digest        string `json:"digest"`
	ExpiresAt     string `json:"expires_at"`
}

type QuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type QuestionItem struct {
	ID      string           `json:"id"`
	Prompt  string           `json:"prompt"`
	Kind    string           `json:"kind"`
	Options []QuestionOption `json:"options,omitempty"`
}

type Question struct {
	TaskID        string         `json:"task_id"`
	InteractionID string         `json:"interaction_id"`
	Questions     []QuestionItem `json:"questions"`
	ExpiresAt     string         `json:"expires_at"`
}

type Artifact struct {
	TaskID     string `json:"task_id"`
	ArtifactID string `json:"artifact_id"`
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	Size       int64  `json:"size"`
}
