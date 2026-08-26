package codex

const threadhallDeveloperInstructions = "Answer as a Threadhall teammate. Do not inspect the filesystem, external services, or other Codex threads. Use only the supplied bounded chat context. For complex requests with at least two independent, bounded subtasks, delegate them in parallel and consolidate their results. Handle short conversational requests directly. When a visual materially improves the answer, use the installed Visualize skill and write a dependency-free HTML fragment only inside the current working directory. Do not return Mermaid because Threadhall renders generated visual fragments inline. Reference every generated HTML or Markdown deliverable with a Markdown link to its local file so Threadhall can persist and preview it."

type threadConfig struct {
	Model                   string
	ReasoningEffort         string
	SubagentModel           string
	SubagentReasoningEffort string
	MaxConcurrentSubagents  int
}

func threadStartParams(cwd string, config threadConfig) map[string]any {
	params := map[string]any{
		"cwd": cwd, "approvalPolicy": "never", "sandbox": "workspace-write", "ephemeral": true,
		"developerInstructions": threadhallDeveloperInstructions,
	}
	if config.Model != "" {
		params["model"] = config.Model
	}
	overrides := map[string]any{}
	if config.ReasoningEffort != "" {
		overrides["model_reasoning_effort"] = config.ReasoningEffort
	}
	if config.SubagentModel != "" || config.SubagentReasoningEffort != "" || config.MaxConcurrentSubagents > 0 {
		agents := map[string]any{"enabled": true}
		if config.SubagentModel != "" {
			agents["default_subagent_model"] = config.SubagentModel
		}
		if config.SubagentReasoningEffort != "" {
			agents["default_subagent_reasoning_effort"] = config.SubagentReasoningEffort
		}
		if config.MaxConcurrentSubagents > 0 {
			agents["max_concurrent_threads_per_session"] = config.MaxConcurrentSubagents
		}
		overrides["agents"] = agents
	}
	if len(overrides) > 0 {
		params["config"] = overrides
	}
	return params
}
