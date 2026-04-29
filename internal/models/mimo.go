/*
 * File: mimo.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package models

type Auth struct {
	Cookie string
	Ph     string
	Token  string
}

type MimoPayload struct {
	MsgID          string      `json:"msgId"`
	ConversationID string      `json:"conversationId"`
	Query          string      `json:"query"`
	IsEditedQuery  bool        `json:"isEditedQuery"`
	ModelConfig    ModelConfig `json:"modelConfig"`
	MultiMedias    []interface{} `json:"multiMedias"`
}

type ModelConfig struct {
	EnableThinking  bool   `json:"enableThinking"`
	WebSearchStatus string `json:"webSearchStatus"`
	Model           string `json:"model"`
}

type MimoResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}
