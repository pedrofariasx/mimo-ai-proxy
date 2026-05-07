/*
 * File: llm.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

import (
	"fmt"
	"mimoproxy/internal/models"
	"mimoproxy/internal/services"
	"mimoproxy/internal/utils"
	"os"
	"strings"
)

func CallLLM(systemPrompt string, userPrompt string, forceJSON bool) (string, error) {
	model := os.Getenv("AGENT_MODEL")
	if model == "" {
		model = "mimo-v2.5-no-thinking"
	}

	if forceJSON {
		userPrompt += "\n\nCRITICAL: Return ONLY valid JSON."
	}

	auth := services.GetSelectedAuth()
	
	payload := models.MimoPayload{
		MsgID:          utils.GenerateID(),
		ConversationID: utils.GenerateID(),
		Query:          fmt.Sprintf("%s\n\n%s", systemPrompt, userPrompt),
		IsEditedQuery:  false,
		ModelConfig: models.ModelConfig{
			EnableThinking:  !strings.Contains(model, "no-thinking"),
			WebSearchStatus: "disabled",
			Model:           model,
		},
	}

	content, err := services.HandleMimoChat(payload, auth)
	if err != nil {
		return "", err
	}

	// Strip markdown JSON blocks if present
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}

	return strings.TrimSpace(content), nil
}
