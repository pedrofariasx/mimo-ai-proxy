/*
 * File: state.go
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
	"mimoproxy/internal/services"
)

func LoadOrInitializeState(goalID string, goal string) (*AgentState, error) {
	_, status, stateJsonStr, err := services.GetAgentState(goalID)
	if err != nil {
		// Does not exist, initialize new
		newState := &AgentState{
			ID:          goalID,
			Goal:        goal,
			PastActions: make([]ActionRecord, 0),
			Results:     make([]string, 0),
			Errors:      make([]string, 0),
			Variables:   make(map[string]string),
			Done:        false,
		}
		err = PersistState(newState)
		if err != nil {
			return nil, err
		}
		return newState, nil
	}

	var state AgentState
	if err := json.Unmarshal([]byte(stateJsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %v", err)
	}

	// Update status logic here if needed
	if status == "done" {
		state.Done = true
	}

	return &state, nil
}

func (s *AgentState) Compact() map[string]interface{} {
	// Limit past actions and results to the last 5 to save tokens
	limit := 5
	pastActions := s.PastActions
	results := s.Results
	
	if len(pastActions) > limit {
		pastActions = pastActions[len(pastActions)-limit:]
	}
	if len(results) > limit {
		results = results[len(results)-limit:]
	}

	return map[string]interface{}{
		"id":           s.ID,
		"goal":         s.Goal,
		"current_task": s.CurrentTask,
		"past_actions": pastActions,
		"results":      results,
		"errors":       s.Errors,
		"variables":    s.Variables,
		"history_truncated": len(s.PastActions) > limit,
	}
}

func PersistState(state *AgentState) error {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	
	status := "running"
	if state.Done {
		status = "done"
	}
	
	return services.SaveAgentState(state.ID, state.Goal, status, string(stateBytes))
}
