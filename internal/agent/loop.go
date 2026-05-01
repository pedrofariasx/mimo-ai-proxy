/*
 * File: loop.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

import (
	"fmt"
	"log"
)

func RunAgentLoop(goalID string, goal string, maxSteps int) (string, error) {
	state, err := LoadOrInitializeState(goalID, goal)
	if err != nil {
		return "", fmt.Errorf("failed to load state: %v", err)
	}

	registry := NewToolRegistry()
	
	if state.Done {
		log.Printf("Agent %s already completed.", goalID)
		return state.FinalOutput, nil
	}

	for step := 1; step <= maxSteps; step++ {
		log.Printf("=== Agent Loop %s - Step %d ===", goalID, step)
		Broadcast(goalID, "step_start", map[string]interface{}{"step": step})
		
		// 1. Planner
		Broadcast(goalID, "planning", nil)
		plan, err := Plan(state)
		if err != nil {
			log.Printf("Planner Error: %v", err)
			state.Errors = append(state.Errors, fmt.Sprintf("Planner Error: %v", err))
			PersistState(state)
			Broadcast(goalID, "error", map[string]interface{}{"error": fmt.Sprintf("Planner Error: %v", err)})
			continue
		}
		
		if len(plan.Tasks) > 0 {
			state.CurrentTask = plan.Tasks[0].Description
		}
		Broadcast(goalID, "plan_updated", plan)

		// 2. Executor Decision
		Broadcast(goalID, "deciding", nil)
		action, err := ExecutorDecide(state, plan, registry)
		if err != nil {
			log.Printf("Executor Decision Error: %v", err)
			state.Errors = append(state.Errors, fmt.Sprintf("Executor Decision Error: %v", err))
			PersistState(state)
			Broadcast(goalID, "error", map[string]interface{}{"error": fmt.Sprintf("Executor Decision Error: %v", err)})
			continue
		}

		// Check for termination
		if action.Action == "finish" {
			log.Printf("Agent finished: %v", action.Input)
			if final, ok := action.Input["final_output"].(string); ok {
				state.FinalOutput = final
			} else {
				state.FinalOutput = "Goal achieved."
			}
			state.Done = true
			PersistState(state)
			Broadcast(goalID, "finished", map[string]interface{}{"final_output": state.FinalOutput})
			break
		}

		// 3. Execution
		log.Printf("Executing action: %s", action.Action)
		Broadcast(goalID, "action_start", action)
		
		result, execErr := registry.ExecuteWithRetry(action.Action, action.Input)
		var resultStr string
		if execErr != nil {
			log.Printf("Execution Failed: %v", execErr)
			resultStr = fmt.Sprintf("Error: %v", execErr)
		} else {
			// truncate result if it's too long
			if len(result) > 2000 {
				resultStr = result[:2000] + "... (truncated)"
			} else {
				resultStr = result
			}
			log.Printf("Execution Result: %s", resultStr)
		}
		Broadcast(goalID, "action_result", map[string]interface{}{"action": action.Action, "result": resultStr})

		// 4. Critic
		Broadcast(goalID, "reflecting", nil)
		reflection, err := Criticize(state, action, resultStr)
		if err != nil {
			log.Printf("Critic Error: %v", err)
			state.Errors = append(state.Errors, fmt.Sprintf("Critic Error: %v", err))
			PersistState(state)
			Broadcast(goalID, "error", map[string]interface{}{"error": fmt.Sprintf("Critic Error: %v", err)})
			continue
		}
		
		log.Printf("Critic Reflection: Success=%v, Analysis=%s", reflection.Success, reflection.Analysis)
		Broadcast(goalID, "reflection", reflection)

		// 5. Update State
		state.PastActions = append(state.PastActions, ActionRecord{
			Action: action.Action,
			Input:  action.Input,
		})
		state.Results = append(state.Results, resultStr)
		if !reflection.Success {
			state.Errors = append(state.Errors, reflection.Analysis)
		}

		// Persist Step
		if err := PersistState(state); err != nil {
			log.Printf("Failed to persist state: %v", err)
		}
		
		if state.Done {
			break
		}
	}

	if !state.Done {
		log.Printf("Agent %s reached max steps (%d) without finishing.", goalID, maxSteps)
		return "", fmt.Errorf("reached max steps")
	}

	return state.FinalOutput, nil
}
