/*
 * File: fingerprint_test.go
 * Project: services
 * Created: 2026-05-06
 *
 * Last Modified: Wed May 06 2026
 * Modified By: Pedro Farias
 */

package services

import (
	"mimoproxy/internal/models"
	"testing"
)

func TestGenerateFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		messages []models.Message
		want     string
	}{
		{
			name: "single user message",
			messages: []models.Message{
				{Role: "user", Content: "hello"},
			},
			want: "sess_757365723a68656c6c6f", // hex for "user:hello"
		},
		{
			name: "system and user message",
			messages: []models.Message{
				{Role: "system", Content: "you are a helpful assistant"},
				{Role: "user", Content: "hello"},
			},
			want: "sess_757365723a68656c6c6f",
		},
		{
			name: "long message truncation",
			messages: []models.Message{
				{Role: "user", Content: "this is a very long message that should be truncated because it exceeds the limit of two hundred characters in the fingerprint generation logic to ensure stability and efficiency in the session management"},
			},
			want: "sess_757365723a7468697320697320612076657279206c6f6e67206d65737361676520746861742073686f756c64206265207472756e63617465642062656361757365206974206578636565647320746865206c696d6974206f662074776f2068756e64726564206368617261637465727320696e207468652066696e6765727072696e742067656e65726174696f6e206c6f67696320746f20656e737572652073746162696c69747920616e6420656666696369656e637920696e207468652073657373696f6e206d",
		},
		{
			name: "no user message fallback",
			messages: []models.Message{
				{Role: "system", Content: "instructions"},
			},
			want: "sess_757365723a696e737472756374696f6e73",
		},
		{
			name: "empty messages",
			messages: []models.Message{},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateFingerprint(tt.messages); got != tt.want {
				t.Errorf("GenerateFingerprint() = %v, want %v", got, tt.want)
			}
		})
	}
}
