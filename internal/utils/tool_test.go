package utils

import (
	"encoding/json"
	"testing"
)

func TestNormalizeMiMoOutputToolCallWithThinking(t *testing.T) {
	raw := "<think>\x00need the weather tool</think>\x00<tool_call>\n{\"name\":\"get_weather\",\"arguments\":{\"city\":\"São Paulo\"}}\n</tool_call>"

	got := NormalizeMiMoOutput(raw, "")
	if got.Content != "" {
		t.Fatalf("content = %q, want empty for tool call", got.Content)
	}
	if got.ReasoningContent != "need the weather tool" {
		t.Fatalf("reasoning = %q", got.ReasoningContent)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("tool name = %q", got.ToolCalls[0].Function.Name)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(got.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("arguments are not valid JSON: %v", err)
	}
	if args["city"] != "São Paulo" {
		t.Fatalf("city = %q", args["city"])
	}
}

func TestNormalizeMiMoOutputFinalContentRemovesControlTags(t *testing.T) {
	raw := "<think>draft answer</think><attempt_completion><result>Resposta final.</result></attempt_completion>"

	got := NormalizeMiMoOutput(raw, "")
	if got.Content != "Resposta final." {
		t.Fatalf("content = %q", got.Content)
	}
	if len(got.ToolCalls) != 0 {
		t.Fatalf("tool calls = %d, want 0", len(got.ToolCalls))
	}
}
