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

type MultiMedia struct {
	MediaType         string `json:"mediaType"`
	FileUrl           string `json:"fileUrl"`
	CompressedVideoUrl string `json:"compressedVideoUrl"`
	AudioTrackUrl      string `json:"audioTrackUrl"`
	Name              string `json:"name"`
	Size              int64  `json:"size"`
	Status            string `json:"status"`
	ObjectName        string `json:"objectName"`
	TokenUsage        int    `json:"tokenUsage"`
	URL               string `json:"url"`
}

type MimoPayload struct {
	MsgID          string      `json:"msgId"`
	ConversationID string      `json:"conversationId"`
	Query          string      `json:"query"`
	IsEditedQuery  bool        `json:"isEditedQuery"`
	ModelConfig    ModelConfig `json:"modelConfig"`
	MultiMedias    []MultiMedia `json:"multiMedias"`
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
