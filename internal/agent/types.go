/*
 * File: types.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

type AgentState struct {
	ID           string            `json:"id"`
	Goal         string            `json:"goal"`
	CurrentTask  string            `json:"current_task"`
	PastActions  []ActionRecord    `json:"past_actions"`
	Results      []string          `json:"results"`
	Errors       []string          `json:"errors"`
	Done         bool              `json:"done"`
	FinalOutput  string            `json:"final_output"`
	Variables    map[string]string `json:"variables"` // For state passing
}

type ActionRecord struct {
	Action string                 `json:"action"`
	Input  map[string]interface{} `json:"input"`
}

type PlanResponse struct {
	Tasks []Task `json:"tasks"`
}

type Task struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
}

type ExecutorDecision struct {
	Action string                 `json:"action"`
	Input  map[string]interface{} `json:"input"`
}

type CriticResponse struct {
	Success            bool   `json:"success"`
	Analysis           string `json:"analysis"`
	NextStepSuggestion string `json:"next_step_suggestion"`
}
