/*
 * File: executor.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

import (
	"encoding/json"
	"fmt"
)

func ExecutorDecide(state *AgentState, plan *PlanResponse, toolsRegistry *ToolRegistry) (*ExecutorDecision, error) {
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	planBytes, _ := json.MarshalIndent(plan, "", "  ")
	
	systemPrompt := fmt.Sprintf(ExecutorSystemPrompt, toolsRegistry.GetToolDescriptions())
	
	userPrompt := fmt.Sprintf("Goal: %s\n\nCurrent State:\n%s\n\nCurrent Plan:\n%s\n\nDecide the next action to take.", state.Goal, string(stateBytes), string(planBytes))
	
	response, err := CallLLM(systemPrompt, userPrompt, true)
	if err != nil {
		return nil, fmt.Errorf("executor failed: %v", err)
	}
	
	var decision ExecutorDecision
	if err := json.Unmarshal([]byte(response), &decision); err != nil {
		return nil, fmt.Errorf("failed to parse executor output: %v\nRaw Output: %s", err, response)
	}
	
	return &decision, nil
}
