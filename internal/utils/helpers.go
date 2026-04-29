/*
 * File: helpers.go
 * Project: mimoproxy
 * Created: 2026-04-29
 *
 * Last Modified: Wed Apr 29 2026
 * Modified By: Pedro Farias
 */

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"mimoproxy/internal/models"
	"time"

	"github.com/gin-gonic/gin"
)

func GenerateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func CreateChatCompletionChunk(id, content, model string, finishReason *string, reasoning string, usage *models.Usage, toolCalls []models.ToolCall) models.ChatCompletionChunk {
	chunk := models.ChatCompletionChunk{
		ID:      "chatcmpl-" + id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.Choice{
			{
				Index: 0,
				Delta: models.Delta{},
				FinishReason: finishReason,
			},
		},
	}

	if content != "" {
		chunk.Choices[0].Delta.Content = content
	}
	if reasoning != "" {
		chunk.Choices[0].Delta.ReasoningContent = reasoning
	}
	if toolCalls != nil {
		chunk.Choices[0].Delta.ToolCalls = toolCalls
	}
	if usage != nil {
		chunk.Usage = usage
	}
	return chunk
}

func SendError(c *gin.Context, status int, message, errorType string, code *string) {
	c.JSON(status, models.ErrorResponse{
		Error: models.ErrorDetail{
			Message: message,
			Type:    errorType,
			Param:   nil,
			Code:    code,
		},
	})
}

func PointerToString(s string) *string {
	return &s
}
