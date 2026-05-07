/*
 * File: planner.go
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

func Plan(state *AgentState) (*PlanResponse, error) {
	stateBytes, _ := json.MarshalIndent(state.Compact(), "", "  ")
	
	userPrompt := fmt.Sprintf("Goal: %s\nCurrent State (Recent History):\n%s\n\nPlease provide a list of tasks to accomplish the goal.", state.Goal, string(stateBytes))
	
	response, err := CallLLM(PlannerSystemPrompt, userPrompt, true)
	if err != nil {
		return nil, fmt.Errorf("planner failed: %v", err)
	}
	
	var plan PlanResponse
	if err := json.Unmarshal([]byte(response), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse planner output: %v\nRaw Output: %s", err, response)
	}
	
	return &plan, nil
}
