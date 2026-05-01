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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func CallLLM(systemPrompt string, userPrompt string, forceJSON bool) (string, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/v1/chat/completions", port)

	if forceJSON {
		userPrompt += "\n\nCRITICAL: Return ONLY valid JSON."
	}

	reqBody := map[string]interface{}{
		"model": "mimo-v2.5-no-thinking",
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": userPrompt,
			},
		},
		"stream": false,
	}

	payload, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse OpenAI-like response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from LLM")
	}

	content := result.Choices[0].Message.Content
	
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
