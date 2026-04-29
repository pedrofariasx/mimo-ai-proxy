/*
 * File: mimo.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"mimoproxy/internal/models"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var MODEL_MAPPING = map[string]string{
	"gpt-4o":        "mimo-v2.5-pro",
	"gpt-4":         "mimo-v2.5-pro",
	"gpt-3.5-turbo": "mimo-v2.5-pro",
	"mimo-pro":      "mimo-v2.5-pro",
	"mimo-search":   "mimo-v2.5-pro",
}

func GetSelectedAuth() models.Auth {
	serviceTokensStr := os.Getenv("SERVICE_TOKENS")
	if serviceTokensStr == "" {
		serviceTokensStr = os.Getenv("SERVICE_TOKEN")
	}
	tokens := strings.Split(serviceTokensStr, ",")

	userIdsStr := os.Getenv("USER_IDS")
	if userIdsStr == "" {
		userIdsStr = os.Getenv("USER_ID")
	}
	userIds := strings.Split(userIdsStr, ",")

	phsStr := os.Getenv("XIAOMI_CHATBOT_PHS")
	if phsStr == "" {
		phsStr = os.Getenv("XIAOMI_CHATBOT_PH")
	}
	phs := strings.Split(phsStr, ",")

	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(tokens))

	selectedToken := strings.TrimSpace(tokens[index])
	selectedUserId := strings.TrimSpace(userIds[index%len(userIds)])
	selectedPh := strings.TrimSpace(phs[index%len(phs)])

	return models.Auth{
		Cookie: fmt.Sprintf("serviceToken=\"%s\"; userId=%s; xiaomichatbot_ph=\"%s\"", selectedToken, selectedUserId, selectedPh),
		Ph:     selectedPh,
		Token:  selectedToken,
	}
}

func ExtractText(content interface{}, stripArtifacts bool) string {
	var text string
	switch v := content.(type) {
	case string:
		text = v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			} else if m, ok := item.(map[string]interface{}); ok {
				mType, _ := m["type"].(string)
				switch mType {
				case "text":
					if t, ok := m["text"].(string); ok {
						parts = append(parts, t)
					}
				case "reasoning":
					if t, ok := m["text"].(string); ok {
						if !stripArtifacts {
							parts = append(parts, fmt.Sprintf("<think>%s</think>", t))
						}
					}
				case "tool_use":
					if !stripArtifacts {
						name, _ := m["name"].(string)
						input := m["input"]
						inputBytes, _ := json.Marshal(input)
						parts = append(parts, fmt.Sprintf("<tool_call>{\"name\": \"%s\", \"arguments\": %s}</tool_call>", name, string(inputBytes)))
					}
				case "tool_result":
					if !stripArtifacts {
						content, _ := m["content"].(string)
						parts = append(parts, fmt.Sprintf("<tool_result>%s</tool_result>", content))
					}
				}
			}
		}
		text = strings.Join(parts, "\n")
	default:
		text = fmt.Sprintf("%v", content)
	}

	if stripArtifacts {
		// Remove <think> blocks
		reThink := regexp.MustCompile(`(?s)<think>.*?</think>`)
		text = reThink.ReplaceAllString(text, "")

		// Remove other tags
		reTags := regexp.MustCompile(`(?s)<(result|attempt_completion|call_id|tool_call|tool_use).*?>`)
		text = reTags.ReplaceAllString(text, "")
	}

	return strings.TrimSpace(text)
}

func GetOfficialHeaders(auth models.Auth, customHeaders map[string]string) map[string]string {
	headers := map[string]string{
		"accept":             "*/*",
		"accept-language":    "system",
		"content-type":       "application/json",
		"cookie":             auth.Cookie,
		"origin":             "https://aistudio.xiaomimimo.com",
		"priority":           "u=1, i",
		"referer":            "https://aistudio.xiaomimimo.com/",
		"sec-ch-ua":          "\"Google Chrome\";v=\"147\", \"Not.A/Brand\";v=\"8\", \"Chromium\";v=\"147\"",
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": "\"Linux\"",
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36",
		"x-timezone":         "America/Maceio",
	}

	if val, ok := customHeaders["accept-language"]; ok {
		headers["accept-language"] = val
	}
	if val, ok := customHeaders["user-agent"]; ok {
		headers["user-agent"] = val
	}
	if val, ok := customHeaders["x-timezone"]; ok {
		headers["x-timezone"] = val
	}

	return headers
}

type DialogItem struct {
	ConversationID string `json:"conversationId"`
	MsgID          string `json:"msgId"`
	InputInfo      struct {
		Query       string        `json:"query"`
		MultiMedias []interface{} `json:"multiMedias"`
	} `json:"inputInfo"`
	CreateTime          string `json:"createTime"`
	DialogLogDetailList []struct {
		Result string `json:"result"`
		Usage  struct {
			TotalTokens int `json:"totalTokens"`
		} `json:"usage"`
	} `json:"dialogLogDetailList"`
}

func GetConversationHistory(auth models.Auth, conversationID string) ([]DialogItem, error) {
	url := fmt.Sprintf("https://aistudio.xiaomimimo.com/open-apis/chat/dialog/list?xiaomichatbot_ph=%s", auth.Ph)

	payload := map[string]interface{}{
		"queryParam": map[string]string{
			"conversationId": conversationID,
		},
		"pageInfo": map[string]int{
			"pageNum":  1,
			"pageSize": 20,
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	headers := GetOfficialHeaders(auth, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Code int          `json:"code"`
		Data []DialogItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

func CreateConversation(auth models.Auth, conversationID string) error {
	url := fmt.Sprintf("https://aistudio.xiaomimimo.com/open-apis/chat/conversation/save?xiaomichatbot_ph=%s", auth.Ph)

	payload := map[string]interface{}{
		"conversationId": conversationID,
		"title":          "New conversation",
		"type":           "chat",
	}

	payloadBytes, _ := json.Marshal(payload)
	headers := GetOfficialHeaders(auth, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Xiaomi returned status %d", resp.StatusCode)
	}

	return nil
}
