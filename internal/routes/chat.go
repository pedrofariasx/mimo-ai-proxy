/*
 * File: chat.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package routes

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mimoproxy/internal/models"
	"mimoproxy/internal/services"
	"mimoproxy/internal/utils"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	TokenStats       = make(map[string]int)
	TokenUsageStats  = make(map[string]int)
	ResponseTimes    = make([]int64, 0)
	StatsMutex       sync.Mutex
)

func AddResponseTime(t int64) {
	StatsMutex.Lock()
	defer StatsMutex.Unlock()
	ResponseTimes = append(ResponseTimes, t)
	if len(ResponseTimes) > 50 {
		ResponseTimes = ResponseTimes[1:]
	}
}

func IncrementTokenStat(token string, tokens int) {
	StatsMutex.Lock()
	defer StatsMutex.Unlock()
	TokenStats[token]++
	TokenUsageStats[token] += tokens
}

func GetStats() (map[string]int, map[string]int, []int64) {
	StatsMutex.Lock()
	defer StatsMutex.Unlock()
	// Return copies
	stats := make(map[string]int)
	for k, v := range TokenStats {
		stats[k] = v
	}
	usage := make(map[string]int)
	for k, v := range TokenUsageStats {
		usage[k] = v
	}
	times := make([]int64, len(ResponseTimes))
	copy(times, ResponseTimes)
	return stats, usage, times
}

func RegisterChatRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc) {
	v1 := r.Group("/v1")
	if authMiddleware != nil {
		v1.Use(authMiddleware)
	}
	
	{
		v1.GET("/models", handleModels)
		v1.POST("/chat/completions", handleChatCompletions)
		v1.GET("/chat/history/:conversationId", handleGetHistory)
		v1.POST("/files", handleFileUpload)
	}

	r.POST("/open-apis/bot/chat", handleDirectProxy)
}

func handleModels(c *gin.Context) {
	if cached, found := services.GlobalCache.Get("models_list"); found {
		c.JSON(http.StatusOK, cached)
		return
	}

	auth := services.GetSelectedAuth()
	headers := services.GetOfficialHeaders(auth, nil)

	req, _ := http.NewRequest("GET", "https://aistudio.xiaomimimo.com/open-apis/bot/config", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := services.GlobalHTTPClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		var result struct {
			Code int `json:"code"`
			Data struct {
				ModelConfigList []struct {
					Model   string `json:"model"`
					EnIntro string `json:"enIntro"`
				} `json:"modelConfigList"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Code == 0 {
			modelsList := make([]map[string]interface{}, 0)
			for _, m := range result.Data.ModelConfigList {
				modelsList = append(modelsList, map[string]interface{}{
					"id":       m.Model,
					"object":   "model",
					"created":  1700000000,
					"owned_by": "xiaomi",
					"description": m.EnIntro,
				})
			}
			response := gin.H{"object": "list", "data": modelsList}
			services.GlobalCache.Set("models_list", response, 30*time.Minute)
			c.JSON(http.StatusOK, response)
			return
		}
	}

	// If API fails and no cache, return empty list or error
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": []interface{}{}})
}

func handleDirectProxy(c *gin.Context) {
	auth := services.GetSelectedAuth()
	url := fmt.Sprintf("https://aistudio.xiaomimimo.com/open-apis/bot/chat?xiaomichatbot_ph=%s", auth.Ph)

	body, _ := io.ReadAll(c.Request.Body)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	
	customHeaders := make(map[string]string)
	for k, v := range c.Request.Header {
		customHeaders[strings.ToLower(k)] = v[0]
	}
	headers := services.GetOfficialHeaders(auth, customHeaders)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := services.GlobalHTTPClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to proxy request", "details": err.Error()})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(respBody, &result)
	c.JSON(resp.StatusCode, result)
}

func handleGetHistory(c *gin.Context) {
	conversationID := c.Param("conversationId")
	syncParam := c.Query("sync") == "true"

	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId is required"})
		return
	}

	var messages []models.Message
	var err error

	if syncParam {
		auth := services.GetSelectedAuth()
		history, err := services.GetConversationHistory(auth, conversationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch history", "details": err.Error()})
			return
		}

		// Convert to OpenAI-like format and SAVE to local DB
		for _, item := range history {
			// User message
			messages = append(messages, models.Message{
				Role:    "user",
				Content: item.InputInfo.Query,
			})
			services.SaveMessage(conversationID, item.MsgID+"_u", "user", item.InputInfo.Query)

			// Assistant message
			if len(item.DialogLogDetailList) > 0 {
				messages = append(messages, models.Message{
					Role:    "assistant",
					Content: item.DialogLogDetailList[0].Result,
				})
				services.SaveMessage(conversationID, item.MsgID+"_a", "assistant", item.DialogLogDetailList[0].Result)
			}
		}
	} else {
		// Try local first
		messages, err = services.GetLocalHistory(conversationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get local history", "details": err.Error()})
			return
		}

		// If empty local, fallback to sync automatically
		if len(messages) == 0 {
			auth := services.GetSelectedAuth()
			history, _ := services.GetConversationHistory(auth, conversationID)
			for _, item := range history {
				messages = append(messages, models.Message{Role: "user", Content: item.InputInfo.Query})
				services.SaveMessage(conversationID, item.MsgID+"_u", "user", item.InputInfo.Query)
				if len(item.DialogLogDetailList) > 0 {
					messages = append(messages, models.Message{Role: "assistant", Content: item.DialogLogDetailList[0].Result})
					services.SaveMessage(conversationID, item.MsgID+"_a", "assistant", item.DialogLogDetailList[0].Result)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"conversationId": conversationID,
		"messages":       messages,
		"source":         "local+synced",
	})
}

func processAutoUploads(messages []models.Message, auth models.Auth) []models.MultiMedia {
	var medias []models.MultiMedia

	if len(messages) == 0 {
		return medias
	}

	// Only process the last message to avoid re-uploading and re-referencing 
	// attachments from previous turns in the history.
	msg := messages[len(messages)-1]

	contentArray, ok := msg.Content.([]interface{})
	if !ok {
		return medias
	}

	for _, item := range contentArray {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		mType, _ := m["type"].(string)
		var data []byte
		var fileName string
		var mediaType string

		if mType == "image_url" {
			urlInfo, _ := m["image_url"].(map[string]interface{})
			url, _ := urlInfo["url"].(string)
			if url == "" {
				continue
			}

			if strings.HasPrefix(url, "data:image/") {
				// Base64
				parts := strings.SplitN(url, ",", 2)
				if len(parts) != 2 {
					continue
				}
				decoded, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					continue
				}
				data = decoded
				
				// Guess extension from mime type
				mime := strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
				ext := "png"
				if strings.Contains(mime, "/") {
					ext = strings.Split(mime, "/")[1]
				}
				fileName = fmt.Sprintf("upload_%d.%s", time.Now().UnixNano(), ext)
				mediaType = "image"
			} else if strings.HasPrefix(url, "http") {
				// External URL
				resp, err := http.Get(url)
				if err != nil {
					continue
				}
				defer resp.Body.Close()
				data, _ = io.ReadAll(resp.Body)
				fileName = fmt.Sprintf("url_upload_%d.png", time.Now().UnixNano())
				mediaType = "image"
			}
		} else if mType == "input_audio" {
			audioInfo, _ := m["input_audio"].(map[string]interface{})
			audioData, _ := audioInfo["data"].(string)
			if audioData != "" {
				decoded, err := base64.StdEncoding.DecodeString(audioData)
				if err != nil {
					continue
				}
				data = decoded
				fileName = fmt.Sprintf("audio_%d.webm", time.Now().UnixNano())
				mediaType = "audio"
			}
		}

		if len(data) > 0 {
			result, err := services.UploadToXiaomi(auth, fileName, data, mediaType)
			if err == nil && result != nil {
				medias = append(medias, *result)
			}
		}
	}

	return medias
}

func handleFileUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "No file uploaded", "invalid_request_error", nil)
		return
	}

	f, err := file.Open()
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to open file", "server_error", nil)
		return
	}
	defer f.Close()

	fileData, err := io.ReadAll(f)
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to read file", "server_error", nil)
		return
	}

	// Determine mediaType
	mediaType := "file"
	// Simpler extension check
	if strings.Contains(file.Header.Get("Content-Type"), "image") {
		mediaType = "image"
	} else if strings.Contains(file.Header.Get("Content-Type"), "audio") || strings.HasSuffix(strings.ToLower(file.Filename), ".webm") {
		mediaType = "audio"
	} else if strings.Contains(file.Header.Get("Content-Type"), "video") {
		mediaType = "video"
	}

	auth := services.GetSelectedAuth()
	result, err := services.UploadToXiaomi(auth, file.Filename, fileData, mediaType)
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to upload to Xiaomi", "server_error", nil)
		fmt.Printf("Upload error: %v\n", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleChatCompletions(c *gin.Context) {
	completionID := utils.GenerateID()
	
	// Request caching/de-duplication
	bodyCopy, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Printf("Error reading request body: %v\n", err)
		utils.SendError(c, http.StatusBadRequest, "Failed to read request body", "invalid_request_error", nil)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyCopy))
	cacheKey := fmt.Sprintf("req_%x", bodyCopy)
	fmt.Printf("Incoming request size: %d bytes\n", len(bodyCopy))
	
	if !strings.Contains(string(bodyCopy), "\"stream\":true") {
		if cached, found := services.GlobalCache.Get(cacheKey); found {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var input struct {
		Messages []models.Message   `json:"messages"`
		Model    string             `json:"model"`
		Stream   bool               `json:"stream"`
		User     string             `json:"user"`
		Tools    []models.Tool      `json:"tools"`
		WebSearch bool              `json:"web_search"`
		MultiMedias []models.MultiMedia `json:"multi_medias"`
	}

	if err = c.ShouldBindJSON(&input); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid request body", "invalid_request_error", nil)
		return
	}

	if len(input.Messages) == 0 {
		utils.SendError(c, http.StatusBadRequest, "Messages array is required and cannot be empty", "invalid_request_error", nil)
		return
	}

	toolInstructions := utils.FormatToolsAsInstructions(input.Tools)

	// Check for pending tool calls in cache (Sequential Tool Calling)
	if input.User != "" {
		if pending, found := services.GlobalCache.Get("pending_tools_" + input.User); found {
			if pendingTools, ok := pending.([]models.ToolCall); ok && len(pendingTools) > 0 {
				// The last message should be a tool result
				lastMsg := input.Messages[len(input.Messages)-1]
				if lastMsg.Role == "tool" {
					// Return the next tool call from cache
					nextTool := pendingTools[0]
					remaining := pendingTools[1:]
					if len(remaining) > 0 {
						services.GlobalCache.Set("pending_tools_"+input.User, remaining, 10*time.Minute)
					} else {
						services.GlobalCache.Delete("pending_tools_" + input.User)
					}

					response := models.ChatCompletionChunk{
						ID:      "chatcmpl-" + completionID,
						Object:  "chat.completion",
						Created: time.Now().Unix(),
						Model:   input.Model,
						Choices: []models.Choice{
							{
								Index: 0,
								Delta: models.Delta{
									Role:      "assistant",
									ToolCalls: []models.ToolCall{nextTool},
								},
								FinishReason: utils.PointerToString("tool_calls"),
							},
						},
					}

					if input.Stream {
						c.Header("Content-Type", "text/event-stream")
						// Initial role
						roleChunk := response
						roleChunk.Choices[0].Delta = models.Delta{Role: "assistant"}
						roleChunk.Choices[0].FinishReason = nil
						b1, _ := json.Marshal(roleChunk)
						c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b1)))
						
						// Tool call
						b2, _ := json.Marshal(response)
						c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b2)))
						c.Writer.WriteString("data: [DONE]\n\n")
						c.Writer.Flush()
						return
					} else {
						// Non-stream OpenAI format
						type NonStreamChoice struct {
							Index        int            `json:"index"`
							Message      models.Delta   `json:"message"`
							FinishReason *string        `json:"finish_reason"`
						}
						type NonStreamResponse struct {
							ID      string             `json:"id"`
							Object  string             `json:"object"`
							Created int64              `json:"created"`
							Model   string             `json:"model"`
							Choices []NonStreamChoice  `json:"choices"`
						}
						ns := NonStreamResponse{
							ID:      response.ID,
							Object:  response.Object,
							Created: response.Created,
							Model:   response.Model,
							Choices: []NonStreamChoice{{Index: 0, Message: response.Choices[0].Delta, FinishReason: response.Choices[0].FinishReason}},
						}
						c.JSON(http.StatusOK, ns)
						return
					}
				} else {
					// User sent a new message, clear pending tools
					services.GlobalCache.Delete("pending_tools_" + input.User)
				}
			}
		}
	}

	var query string
	convID := input.User
	var sessionKey string
	
	// Automatic Media Upload
	currentAuth := services.GetSelectedAuth()
	autoMedias := processAutoUploads(input.Messages, currentAuth)
	if len(autoMedias) > 0 {
		input.MultiMedias = append(input.MultiMedias, autoMedias...)
	}

	// AUTOMATIC SESSION DETECTION: If User ID is empty, try to identify conversation by history hash
	if convID == "" && len(input.Messages) > 0 {
		// Find the first user message to create a fingerprint (skip system prompts)
		var firstUserMsgIdx int = -1
		for i, msg := range input.Messages {
			if msg.Role == "user" {
				firstUserMsgIdx = i
				break
			}
		}
		// Fallback: if no user message found, use the last message
		if firstUserMsgIdx == -1 {
			firstUserMsgIdx = len(input.Messages) - 1
		}
		// Create a fingerprint based on the user's text, not system prompts
		firstMsg := "user:" + services.ExtractText(input.Messages[firstUserMsgIdx].Content, true)
		if len(firstMsg) > 200 {
			firstMsg = firstMsg[:200]
		}
		sessionKey = fmt.Sprintf("sess_%x", firstMsg)
		
		// 1. Try Memory Cache
		if cachedID, found := services.GlobalCache.Get(sessionKey); found {
			convID = cachedID.(string)
			fmt.Printf("Detected existing session via memory fingerprint: %s\n", convID)
		} else {
			// 2. Try Database (Sessions table)
			dbID, err := services.GetSession(sessionKey)
			if err == nil && dbID != "" {
				convID = dbID
				services.GlobalCache.Set(sessionKey, convID, 24*time.Hour)
				fmt.Printf("Detected existing session via database fingerprint: %s\n", convID)
			} else {
				// 3. Try Deep Recovery (Messages table fallback)
				firstMsgText := services.ExtractText(input.Messages[firstUserMsgIdx].Content, false)
				deepID, err := services.FindSessionByMessage("user", firstMsgText)
				if err == nil && deepID != "" {
					convID = deepID
					services.GlobalCache.Set(sessionKey, convID, 24*time.Hour)
					_ = services.SaveSession(sessionKey, convID)
					fmt.Printf("Recovered existing session from message history: %s\n", convID)
				} else {
					// 4. First time seeing this conversation fingerprint
					convID = utils.GenerateID()
					services.GlobalCache.Set(sessionKey, convID, 24*time.Hour)
					_ = services.SaveSession(sessionKey, convID)
					
					// Register the new conversation ID with Xiaomi
					if err := services.CreateConversation(currentAuth, convID); err != nil {
						fmt.Printf("Failed to register conversation with Xiaomi: %v\n", err)
					}
					fmt.Printf("Started and registered new session for fingerprint: %s\n", convID)
				}
			}
		}
	}

	// AUTOMATIC SESSION REBOOT: If conversation failed 3+ times, start a new one with a summary
	var summaryContext string
	if convID != "" {
		needsRebootKey := "needs_reboot_" + convID
		if val, found := services.GlobalCache.Get(needsRebootKey); found && val.(bool) {
			fmt.Printf("Rebooting session %s due to consecutive failures...\n", convID)
			summaryContext = services.GenerateSummary(currentAuth, convID)
			
			// Generate new conversation ID
			convID = utils.GenerateID()
			
			// Register new one
			_ = services.CreateConversation(currentAuth, convID)
			
			// If we had a fingerprint, update it to point to the new convID
			if sessionKey != "" {
				services.GlobalCache.Set(sessionKey, convID, 24*time.Hour)
				_ = services.SaveSession(sessionKey, convID)
			}
			
			// Reset failure stats for the old and new (just in case) convIDs
			services.GlobalCache.Delete(needsRebootKey)
			services.GlobalCache.Delete("fail_count_" + convID) 
		}
	}

	// OPTIMIZATION: If we have a Conversation ID, we rely on Xiaomi's server-side state.
	// We only send the last message to avoid hitting the 128KB payload limit for the 'query' field.
	if convID != "" {
		// Sync local history if empty
		localMsgs, _ := services.GetLocalHistory(convID)
		if len(localMsgs) == 0 {
			// Register/Save the conversation ID with Xiaomi first to be safe
			_ = services.CreateConversation(currentAuth, convID)

			remoteHistory, err := services.GetConversationHistory(currentAuth, convID)
			if err == nil && len(remoteHistory) > 0 {
				for _, item := range remoteHistory {
					services.SaveMessage(convID, item.MsgID+"_u", "user", item.InputInfo.Query)
					if len(item.DialogLogDetailList) > 0 {
						services.SaveMessage(convID, item.MsgID+"_a", "assistant", item.DialogLogDetailList[0].Result)
					}
				}
			}
		}

		lastMessage := input.Messages[len(input.Messages)-1]
		lastMessageText := utils.FormatMessageForMiMo(lastMessage)
		
		// Prepend summary if this is a rebooted session
		if summaryContext != "" {
			lastMessageText = summaryContext + lastMessageText
		}
		
		services.SaveMessage(convID, "user_"+utils.GenerateID(), "user", lastMessageText)
		
		var systemContent string
		for _, m := range input.Messages {
			if m.Role == "system" {
				systemContent = services.ExtractText(m.Content, false)
				break
			}
		}

		if systemContent != "" {
			query = fmt.Sprintf("%s%s\n\n%s", systemContent, toolInstructions, lastMessageText)
		} else if toolInstructions != "" {
			query = fmt.Sprintf("System: %s\n\n%s", strings.TrimSpace(toolInstructions), lastMessageText)
		} else {
			query = lastMessageText
		}
		
		// Add a final reminder at the very end of the user's message to ensure strict tool usage
		if toolInstructions != "" && !strings.Contains(lastMessageText, "SYSTEM REMINDER") {
			query += "\n\n[SYSTEM REMINDER: You must ONLY respond with a `<tool_call>` XML block if you need to take an action, or use `attempt_completion` if finished. Do NOT output conversational text.]"
		}
	} else if len(input.Messages) <= 1 {
		lastMessage := input.Messages[len(input.Messages)-1]
		query = utils.FormatMessageForMiMo(lastMessage)
		if summaryContext != "" {
			query = summaryContext + query
		}
	} else {
		// Do not limit history unless it exceeds 1M tokens (~4M characters)
		var processedMessages []string
		var systemPrompt string
		
		// Find system prompt first
		for _, m := range input.Messages {
			if m.Role == "system" {
				systemPrompt = services.ExtractText(m.Content, false) + toolInstructions
				break
			}
		}

		// Include all messages except system (which is handled separately)
		for _, m := range input.Messages {
			if m.Role == "system" {
				continue
			}
			processedMessages = append(processedMessages, utils.FormatMessageForMiMo(m))
		}

		if systemPrompt != "" {
			query = systemPrompt + "\n\n" + strings.Join(processedMessages, "\n\n")
		} else {
			if toolInstructions != "" {
				query = strings.TrimSpace(toolInstructions) + "\n\n" + strings.Join(processedMessages, "\n\n")
			} else {
				query = strings.Join(processedMessages, "\n\n")
			}
		}
		
		if toolInstructions != "" {
			query += "\n\n[SYSTEM REMINDER: You must ONLY respond with a `<tool_call>` XML block if you need to take an action, or use `attempt_completion` if finished. Do NOT output conversational text.]"
		}

		// Only truncate if we exceed the safety limit for payload stability
		// Mimo officially supports 1M tokens (~4M characters)
		maxChars := 4000000
		if len(query) > maxChars {
			// Find a safe point to truncate (after the system prompt)
			// to keep the most recent context.
			
			// Take the last portion of the query
			truncated := query[len(query)-maxChars:]
			
			// Try to find the first newline to avoid starting in middle of a word
			if idx := strings.Index(truncated, "\n"); idx != -1 {
				truncated = truncated[idx+1:]
			}

			if systemPrompt != "" && !strings.Contains(truncated, systemPrompt[:10]) {
				query = systemPrompt + "\n\n... (context truncated) ...\n\n" + truncated
			} else {
				query = truncated
			}

			// Final safety check
			if len(query) > 4100000 {
				query = query[:4100000]
			}
		}
	}

	targetModel := input.Model

	enableThinking := !strings.Contains(input.Model, "no-thinking")
	webSearchStatus := "disabled"
	if strings.Contains(input.Model, "search") || input.WebSearch {
		webSearchStatus = "enabled"
	}

	payload := models.MimoPayload{
		MsgID:          utils.GenerateID(),
		ConversationID: convID,
		Query:          query,
		IsEditedQuery:  false,
		ModelConfig: models.ModelConfig{
			EnableThinking:  enableThinking,
			WebSearchStatus: webSearchStatus,
			Model:           targetModel,
		},
		MultiMedias: []models.MultiMedia{},
	}
	if len(input.MultiMedias) > 0 {
		payload.MultiMedias = input.MultiMedias
	}
	if payload.ConversationID == "" {
		payload.ConversationID = utils.GenerateID()
	}

	// Retry logic
	tokensStr := os.Getenv("SERVICE_TOKENS")
	if tokensStr == "" {
		tokensStr = os.Getenv("SERVICE_TOKEN")
	}
	tokens := strings.Split(tokensStr, ",")
	maxRetries := len(tokens)
	if maxRetries > 3 {
		maxRetries = 3
	}

	var resp *http.Response
	var auth models.Auth

	for i := 0; i < maxRetries; i++ {
		auth = services.GetSelectedAuth()
		url := fmt.Sprintf("https://aistudio.xiaomimimo.com/open-apis/bot/chat?xiaomichatbot_ph=%s", auth.Ph)
		
		payloadBytes, _ := json.Marshal(payload)
		fmt.Printf("[%s] Chat Request: %d bytes | Model: %s | Media: %d\n", 
			completionID, len(payloadBytes), payload.ModelConfig.Model, len(payload.MultiMedias))
		
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
		customHeaders := make(map[string]string)
		for k, v := range c.Request.Header {
			customHeaders[strings.ToLower(k)] = v[0]
		}
		headers := services.GetOfficialHeaders(auth, customHeaders)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		startTime := time.Now()
		resp, err = services.GlobalHTTPClient.Do(req)
		if err == nil {
			fmt.Printf("Xiaomi Response Status: %s (Duration: %v)\n", resp.Status, time.Since(startTime))
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Xiaomi returned non-200 status: %d\n", resp.StatusCode)
				// If not 200, we might want to retry or just fail
				if resp.StatusCode >= 500 {
					resp.Body.Close()
					continue
				}
				// For 4xx, just report it
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				c.JSON(resp.StatusCode, gin.H{"error": "Xiaomi API error", "status": resp.StatusCode, "details": string(body)})
				return
			}
			AddResponseTime(time.Since(startTime).Milliseconds())
			break
		}
		
		fmt.Printf("Error calling Xiaomi (retry %d): %v\n", i, err)
		if i == maxRetries-1 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to proxy request", "details": err.Error()})
			return
		}
	}
	defer resp.Body.Close()

	// Handle potential Gzip response from Xiaomi
	var bodyReader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			bodyReader = gz
		}
	}

	if input.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		// Send initial role chunk
		initialChunk := models.ChatCompletionChunk{
			ID:      "chatcmpl-" + completionID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   targetModel,
			Choices: []models.Choice{
				{
					Index: 0,
					Delta: models.Delta{Role: "assistant"},
				},
			},
		}
		initialBytes, _ := json.Marshal(initialChunk)
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(initialBytes)))
		c.Writer.Flush()

		processStream(c, bodyReader, completionID, targetModel, payload.ConversationID, query)
	} else {
		processNonStream(c, bodyReader, completionID, targetModel, cacheKey, payload.ConversationID, query)
	}
}

func processStream(c *gin.Context, body io.Reader, completionID, model string, userID string, query string) {
	// Use a very large buffer for the reader to handle massive SSE events
	reader := bufio.NewReaderSize(body, 16*1024*1024) // 16MB buffer
	
	var inThinking bool
	var inToolCall bool
	var sentToolCallName bool
	var currentToolID string
	var toolCallIndex int
	var toolCallBuffer strings.Builder
	var fullText strings.Builder
	var reasoningText strings.Builder
	var usage models.Usage

	var eventType string
	var dataStr string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Reader error: %v\n", err)
			}
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataStr = strings.TrimSpace(line[5:])
			
			// Process event
			processEvent(c, eventType, dataStr, completionID, model, true, &inThinking, &inToolCall, &sentToolCallName, &currentToolID, &toolCallIndex, &toolCallBuffer, &fullText, &reasoningText, &usage)
			
			eventType = ""
			dataStr = ""
		}
	}

	// End of stream
	toolCallStr, toolCalls := utils.ParseToolCalls(fullText.String())
	_ = toolCallStr
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	// Ensure usage is at least estimated if missing
	if usage.TotalTokens == 0 {
		usage.PromptTokens = len(query) / 4
		usage.CompletionTokens = len(fullText.String()) / 4
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	IncrementTokenStat(os.Getenv("SERVICE_TOKEN"), usage.TotalTokens) // Use first token or specific one

	// Save assistant message to local history
	services.SaveMessage(userID, "asst_"+completionID, "assistant", fullText.String())

	// Update failure statistics
	updateConversationFailStats(userID, fullText.String())

	finalChunk := utils.CreateChatCompletionChunk(completionID, "", model, &finishReason, "", &usage, nil)
	finalBytes, _ := json.Marshal(finalChunk)
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(finalBytes)))
	c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

func processNonStream(c *gin.Context, body io.Reader, completionID, model string, cacheKey string, userID string, query string) {
	respBody, _ := io.ReadAll(body)
	events := strings.Split(string(respBody), "\n\n")

	var inThinking bool
	var inToolCall bool
	var sentToolCallName bool
	var currentToolID string
	var toolCallIndex int
	var toolCallBuffer strings.Builder
	var fullText strings.Builder
	var reasoningText strings.Builder
	var usage models.Usage

	for _, event := range events {
		if strings.TrimSpace(event) == "" {
			continue
		}
		lines := strings.Split(event, "\n")
		var eventType string
		var dataStr string
		for _, line := range lines {
			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(line[6:])
			} else if strings.HasPrefix(line, "data:") {
				dataStr = strings.TrimSpace(line[5:])
			}
		}
		if dataStr != "" {
			processEvent(c, eventType, dataStr, completionID, model, false, &inThinking, &inToolCall, &sentToolCallName, &currentToolID, &toolCallIndex, &toolCallBuffer, &fullText, &reasoningText, &usage)
		}
	}

	cleanText, toolCalls := utils.ParseToolCalls(fullText.String())
	
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		// If multi-tool and we have a user ID, store the rest in cache
		if userID != "" && len(toolCalls) > 1 {
			services.GlobalCache.Set("pending_tools_"+userID, toolCalls[1:], 10*time.Minute)
		}
	}

	// Ensure usage is at least estimated if missing
	if usage.TotalTokens == 0 {
		usage.PromptTokens = len(query) / 4
		usage.CompletionTokens = fullText.Len() / 4
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	IncrementTokenStat(os.Getenv("SERVICE_TOKEN"), usage.TotalTokens)

	response := models.ChatCompletionChunk{
		ID:      "chatcmpl-" + completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.Choice{
			{
				Index: 0,
				Delta: models.Delta{
					Role:             "assistant",
					Content:          cleanText,
					ReasoningContent: strings.TrimSpace(reasoningText.String()),
					ToolCalls:        toolCalls,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: &usage,
	}

	// Fix Choice for non-streaming: OpenAI uses 'message' instead of 'delta'
	type NonStreamChoice struct {
		Index        int            `json:"index"`
		Message      models.Delta   `json:"message"`
		FinishReason *string        `json:"finish_reason"`
	}
	type NonStreamResponse struct {
		ID      string             `json:"id"`
		Object  string             `json:"object"`
		Created int64              `json:"created"`
		Model   string             `json:"model"`
		Choices []NonStreamChoice  `json:"choices"`
		Usage   *models.Usage      `json:"usage"`
	}

	nsResponse := NonStreamResponse{
		ID:      response.ID,
		Object:  response.Object,
		Created: response.Created,
		Model:   response.Model,
		Choices: []NonStreamChoice{
			{
				Index: 0,
				Message: response.Choices[0].Delta,
				FinishReason: response.Choices[0].FinishReason,
			},
		},
		Usage: response.Usage,
	}

	// Save assistant message to local history
	services.SaveMessage(userID, "asst_"+completionID, "assistant", fullText.String())

	// Update failure statistics
	updateConversationFailStats(userID, fullText.String())

	// Cache successful non-streaming response
	services.GlobalCache.Set(cacheKey, nsResponse, 5*time.Minute)
	c.JSON(http.StatusOK, nsResponse)
}

var (
	reToolName      = regexp.MustCompile(`"name":\s*"([^"]+)"`)
	reToolArgsStart = regexp.MustCompile(`[{\["tfn\d]`)
	reTrailingBrace = regexp.MustCompile(`\s*}$`)
)

func processEvent(c *gin.Context, eventType, dataStr, completionID, model string, isStreaming bool, inThinking, inToolCall, sentToolCallName *bool, currentToolID *string, toolCallIndex *int, toolCallBuffer, fullText, reasoningText *strings.Builder, usage *models.Usage) {
	if eventType == "usage" {
		var u struct {
			PromptTokens     int `json:"promptTokens"`
			CompletionTokens int `json:"completionTokens"`
			TotalTokens      int `json:"totalTokens"`
			NativeUsage      struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"nativeUsage"`
		}
		if err := json.Unmarshal([]byte(dataStr), &u); err == nil {
			if u.PromptTokens > 0 {
				usage.PromptTokens = u.PromptTokens
				usage.CompletionTokens = u.CompletionTokens
				usage.TotalTokens = u.TotalTokens
			} else {
				usage.PromptTokens = u.NativeUsage.PromptTokens
				usage.CompletionTokens = u.NativeUsage.CompletionTokens
				usage.TotalTokens = u.NativeUsage.TotalTokens
			}
		}
		return
	}

	if eventType != "message" {
		return
	}

	var d struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(dataStr), &d); err != nil {
		return
	}

	content := strings.ReplaceAll(d.Content, "\x00", "")
	remaining := content

	for len(remaining) > 0 {
		if *inThinking {
			endIdx := strings.Index(remaining, "</think>")
			if endIdx != -1 {
				text := remaining[:endIdx]
				reasoningText.WriteString(text)
				if isStreaming {
					chunk := utils.CreateChatCompletionChunk(completionID, "", model, nil, text, nil, nil)
					b, _ := json.Marshal(chunk)
					c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
					c.Writer.Flush()
				}
				*inThinking = false
				remaining = remaining[endIdx+8:]
			} else {
				reasoningText.WriteString(remaining)
				if isStreaming {
					chunk := utils.CreateChatCompletionChunk(completionID, "", model, nil, remaining, nil, nil)
					b, _ := json.Marshal(chunk)
					c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
					c.Writer.Flush()
				}
				remaining = ""
			}
		} else if *inToolCall {
			endIdx := strings.Index(remaining, "</tool_call>")
			contentToProcess := remaining
			if endIdx != -1 {
				contentToProcess = remaining[:endIdx]
			}

			toolCallBuffer.WriteString(contentToProcess)

			if isStreaming {
				if !*sentToolCallName {
					bufferStr := toolCallBuffer.String()
					nameMatch := reToolName.FindStringSubmatch(bufferStr)
					argsStartIdx := strings.Index(bufferStr, "\"arguments\":")

					if len(nameMatch) > 1 && argsStartIdx != -1 {
						name := nameMatch[1]
						*currentToolID = "call_" + utils.GenerateID()
						*sentToolCallName = true

						afterArgs := bufferStr[argsStartIdx+12:]
						firstValIdx := reToolArgsStart.FindStringIndex(afterArgs)

						initialToolCalls := []models.ToolCall{
							{
								Index: *toolCallIndex,
								ID:    *currentToolID,
								Type:  "function",
								Function: models.ToolFunction{
									Name:      name,
									Arguments: "",
								},
							},
						}
						chunk := utils.CreateChatCompletionChunk(completionID, "", model, nil, "", nil, initialToolCalls)
						b, _ := json.Marshal(chunk)
						c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))

						if firstValIdx != nil {
							initialArgs := strings.TrimSpace(afterArgs[firstValIdx[0]:])
							if endIdx != -1 {
								initialArgs = reTrailingBrace.ReplaceAllString(initialArgs, "")
							}
							if initialArgs != "" {
								argChunk := []models.ToolCall{
									{
										Index: *toolCallIndex,
										Function: models.ToolFunction{
											Arguments: initialArgs,
										},
									},
								}
								chunk := utils.CreateChatCompletionChunk(completionID, "", model, nil, "", nil, argChunk)
								b, _ := json.Marshal(chunk)
								c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
							}
						}
						c.Writer.Flush()
					}
				} else {
					delta := contentToProcess
					if endIdx != -1 {
						delta = reTrailingBrace.ReplaceAllString(strings.TrimSpace(delta), "")
					}
					if delta != "" {
						argChunk := []models.ToolCall{
							{
								Index: *toolCallIndex,
								Function: models.ToolFunction{
									Arguments: delta,
								},
							},
						}
						chunk := utils.CreateChatCompletionChunk(completionID, "", model, nil, "", nil, argChunk)
						b, _ := json.Marshal(chunk)
						c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
						c.Writer.Flush()
					}
				}
			}

			if endIdx != -1 {
				fullText.WriteString("<tool_call>")
				fullText.WriteString(toolCallBuffer.String())
				fullText.WriteString("</tool_call>")
				*inToolCall = false
				*sentToolCallName = false
				*toolCallIndex++
				toolCallBuffer.Reset()
				remaining = remaining[endIdx+12:]
			} else {
				remaining = ""
			}
		} else {
			thinkStartIdx := strings.Index(remaining, "<think>")
			toolStartIdx := strings.Index(remaining, "<tool_call>")

			if thinkStartIdx != -1 && (toolStartIdx == -1 || thinkStartIdx < toolStartIdx) {
				text := remaining[:thinkStartIdx]
				fullText.WriteString(text)
				if isStreaming && text != "" {
					chunk := utils.CreateChatCompletionChunk(completionID, text, model, nil, "", nil, nil)
					b, _ := json.Marshal(chunk)
					c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
					c.Writer.Flush()
				}
				*inThinking = true
				remaining = remaining[thinkStartIdx+7:]
			} else if toolStartIdx != -1 {
				text := remaining[:toolStartIdx]
				fullText.WriteString(text)
				if isStreaming && text != "" {
					chunk := utils.CreateChatCompletionChunk(completionID, text, model, nil, "", nil, nil)
					b, _ := json.Marshal(chunk)
					c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
					c.Writer.Flush()
				}
				*inToolCall = true
				toolCallBuffer.Reset()
				remaining = remaining[toolStartIdx+11:]
			} else {
				fullText.WriteString(remaining)
				if isStreaming {
					chunk := utils.CreateChatCompletionChunk(completionID, remaining, model, nil, "", nil, nil)
					b, _ := json.Marshal(chunk)
					c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(b)))
					c.Writer.Flush()
				}
				remaining = ""
			}
		}
	}
}

func updateConversationFailStats(userID string, fullText string) {
	if userID == "" {
		return
	}
	
	failCountKey := "fail_count_" + userID
	needsRebootKey := "needs_reboot_" + userID
	
	if len(strings.TrimSpace(fullText)) == 0 {
		count := 0
		if val, found := services.GlobalCache.Get(failCountKey); found {
			count = val.(int)
		}
		count++
		services.GlobalCache.Set(failCountKey, count, 24*time.Hour)
		fmt.Printf("Consecutive empty responses for %s: %d\n", userID, count)
		
		if count >= 3 {
			services.GlobalCache.Set(needsRebootKey, true, 24*time.Hour)
			fmt.Printf("Session %s flagged for reboot due to 3+ failures\n", userID)
		}
	} else {
		// Reset counter on success
		services.GlobalCache.Delete(failCountKey)
		services.GlobalCache.Delete(needsRebootKey)
	}
}
