/*
 * File: critic.go
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

func Criticize(state *AgentState, action *ExecutorDecision, result string) (*CriticResponse, error) {
	actionBytes, _ := json.MarshalIndent(action, "", "  ")
	
	userPrompt := fmt.Sprintf("Goal: %s\n\nAction Taken:\n%s\n\nResult/Output:\n%s\n\nEvaluate the result and provide feedback.", state.Goal, string(actionBytes), result)
	
	response, err := CallLLM(CriticSystemPrompt, userPrompt, true)
	if err != nil {
		return nil, fmt.Errorf("critic failed: %v", err)
	}
	
	var criticResponse CriticResponse
	if err := json.Unmarshal([]byte(response), &criticResponse); err != nil {
		return nil, fmt.Errorf("failed to parse critic output: %v\nRaw Output: %s", err, response)
	}
	
	return &criticResponse, nil
}
