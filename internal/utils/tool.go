/*
 * File: tool.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package utils

import (
	"encoding/json"
	"fmt"
	"mimoproxy/internal/models"
	"regexp"
	"strings"
)

/**
 * Converts OpenAI tool definitions into textual instructions for the system prompt.
 */
func FormatToolsAsInstructions(tools []models.Tool) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n# External Tools\n\n")
	sb.WriteString("You have access to the following tools. To execute a tool, you MUST use the exact XML tag `<tool_call>` with a JSON payload inside. NEVER wrap the JSON in Markdown code blocks (like ```json).\n\n")
	sb.WriteString("Format:\n")
	sb.WriteString("<tool_call>\n{\"name\": \"function_name\", \"arguments\": {\"arg1\": \"value1\"}}\n</tool_call>\n\n")
	sb.WriteString("CRITICAL RULES:\n")
	sb.WriteString("1. If you need to use a tool, output ONLY the `<tool_call>` block. Do NOT include any normal text explaining what you are doing. Do NOT output conversational text.\n")
	sb.WriteString("2. You can only use ONE tool per response.\n")
	sb.WriteString("3. Wait for the tool result before proceeding to the next step.\n\n")
	sb.WriteString("Available tools:\n")

	for _, tool := range tools {
		if tool.Type == "function" {
			funcDef := tool.Function
			sb.WriteString(fmt.Sprintf("\n- %s: %s\n", funcDef.Name, funcDef.Description))
			params, _ := json.Marshal(funcDef.Parameters)
			sb.WriteString(fmt.Sprintf("  Parameters: %s\n", string(params)))
		}
	}

	return sb.String()
}

/**
 * Parses XML-style tool calls from a text string and converts them to OpenAI format.
 */
func ParseToolCalls(text string) (string, []models.ToolCall) {
	var toolCalls []models.ToolCall
	cleanText := text

	// Regex to find <tool_call>{...}</tool_call>
	toolCallRegex := regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	matches := toolCallRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		jsonStr := strings.TrimSpace(match[1])
		// Remove potential markdown wrappers like ```json ... ``` inside the tag
		jsonStr = regexp.MustCompile("(?s)^```[a-z]*\n").ReplaceAllString(jsonStr, "")
		jsonStr = regexp.MustCompile("(?s)\n```$").ReplaceAllString(jsonStr, "")
		jsonStr = strings.TrimSpace(jsonStr)

		var toolCallData struct {
			Name      string      `json:"name"`
			Arguments interface{} `json:"arguments"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &toolCallData); err == nil && toolCallData.Name != "" {
			var argsStr string
			switch v := toolCallData.Arguments.(type) {
			case string:
				argsStr = v
			default:
				b, _ := json.Marshal(v)
				argsStr = string(b)
			}

			toolCalls = append(toolCalls, models.ToolCall{
				ID:   "call_" + GenerateID(),
				Type: "function",
				Function: models.ToolFunction{
					Name:      toolCallData.Name,
					Arguments: argsStr,
				},
			})
			cleanText = strings.Replace(cleanText, match[0], "", 1)
		} else {
			// Try alternate format: <tool_name>JSON_ARGS (closing tag optional)
			altRegex := regexp.MustCompile(`(?s)<(\w+)>(.*)`)
			altMatch := altRegex.FindStringSubmatch(jsonStr)
			if len(altMatch) >= 3 {
				toolName := altMatch[1]
				argsStr := strings.TrimSpace(altMatch[2])
				// Remove trailing tag if present (e.g. </read_file>)
				closeTag := fmt.Sprintf("</%s>", toolName)
				argsStr = strings.TrimSuffix(strings.TrimSpace(argsStr), closeTag)
				argsStr = strings.TrimSpace(argsStr)
				
				toolCalls = append(toolCalls, models.ToolCall{
					ID:   "call_" + GenerateID(),
					Type: "function",
					Function: models.ToolFunction{
						Name:      toolName,
						Arguments: argsStr,
					},
				})
				cleanText = strings.Replace(cleanText, match[0], "", 1)
			}
		}
	}

	// Robustness check for whole JSON or JSON in Markdown block
	if len(toolCalls) == 0 {
		trimmedText := strings.TrimSpace(text)
		
		// Extract json from markdown block if present
		jsonBlockRegex := regexp.MustCompile(`(?s)\x60\x60\x60(?:json)?\s*({.*?})\s*\x60\x60\x60`)
		jsonMatch := jsonBlockRegex.FindStringSubmatch(trimmedText)
		if len(jsonMatch) >= 2 {
			trimmedText = jsonMatch[1]
		}
		
		if strings.HasPrefix(trimmedText, "{") && strings.HasSuffix(trimmedText, "}") {
			var toolCallData struct {
				Name      string      `json:"name"`
				Arguments interface{} `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(trimmedText), &toolCallData); err == nil && toolCallData.Name != "" && toolCallData.Arguments != nil {
				var argsStr string
				switch v := toolCallData.Arguments.(type) {
				case string:
					argsStr = v
				default:
					b, _ := json.Marshal(v)
					argsStr = string(b)
				}

				toolCalls = append(toolCalls, models.ToolCall{
					ID:   "call_" + GenerateID(),
					Type: "function",
					Function: models.ToolFunction{
						Name:      toolCallData.Name,
						Arguments: argsStr,
					},
				})
				
				// If we successfully parsed a fallback tool call, we clear the text
				// so the conversational text doesn't leak out as content.
				cleanText = ""
			}
		}
	}

	return strings.TrimSpace(cleanText), toolCalls
}

/**
 * Converts an OpenAI message back to a string format that MiMo understands.
 */
func FormatMessageForMiMo(message models.Message) string {
	var parts []string

	// Handle tool results (as a separate message or as parts)
	if message.Role == "tool" {
		contentStr := ""
		switch v := message.Content.(type) {
		case string:
			contentStr = v
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "text" {
						if text, ok := m["text"].(string); ok {
							contentStr += text
						}
					}
				}
			}
		}
		return fmt.Sprintf("\n<tool_result>\n%s\n</tool_result>\n\n[SYSTEM REMINDER: You must ONLY respond with a `<tool_call>` XML block if you need to take an action, or use `attempt_completion` if finished. Do NOT output conversational text.]\n", contentStr)
	}

	// Handle normal content and complex parts
	if message.Content != nil {
		switch v := message.Content.(type) {
		case string:
			parts = append(parts, v)
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					mType, _ := m["type"].(string)
					switch mType {
					case "text":
						if text, ok := m["text"].(string); ok {
							parts = append(parts, text)
						}
					case "reasoning":
						if text, ok := m["text"].(string); ok {
							parts = append(parts, fmt.Sprintf("<think>%s</think>", text))
						}
					case "tool_use":
						name, _ := m["name"].(string)
						input := m["input"]
						inputBytes, _ := json.Marshal(input)
						parts = append(parts, fmt.Sprintf("<tool_call>{\"name\": \"%s\", \"arguments\": %s}</tool_call>", name, string(inputBytes)))
					case "tool_result":
						content, _ := m["content"].(string)
						parts = append(parts, fmt.Sprintf("<tool_result>%s</tool_result>", content))
					}
				}
			}
		}
	}

	// Handle tool calls
	if len(message.ToolCalls) > 0 {
		for _, tc := range message.ToolCalls {
			if tc.Type == "function" {
				var args interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				argsBytes, _ := json.Marshal(args)
				parts = append(parts, fmt.Sprintf("<tool_call>{\"name\": \"%s\", \"arguments\": %s}</tool_call>", tc.Function.Name, string(argsBytes)))
			}
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
