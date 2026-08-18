package llmtools

import (
	"fmt"
	"strings"
	"testing"
)

func TestRegistryRegisterAndInvoke(t *testing.T) {
	r := NewRegistry()
	RegisterWeather(r)

	if !r.Has("get_weather") {
		t.Fatal("get_weather should be registered")
	}
	if r.Has("nonexistent") {
		t.Fatal("nonexistent should not be registered")
	}

	result, err := r.Invoke("get_weather", map[string]interface{}{"location": "Дерибасовская"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "На Дерибасовская хорошая погода" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRegistryInvokeNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Invoke("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention tool name: %v", err)
	}
}

func TestSystemPromptSuffix(t *testing.T) {
	r := NewRegistry()
	RegisterWeather(r)

	suffix := r.SystemPromptSuffix()
	if suffix == "" {
		t.Fatal("suffix should not be empty")
	}
	if !strings.Contains(suffix, "get_weather") {
		t.Errorf("suffix should contain tool name: %s", suffix)
	}
	if !strings.Contains(suffix, "tool_call") {
		t.Errorf("suffix should contain tool_call format: %s", suffix)
	}
}

func TestSystemPromptSuffixEmpty(t *testing.T) {
	r := NewRegistry()
	if suffix := r.SystemPromptSuffix(); suffix != "" {
		t.Errorf("empty registry should return empty suffix, got: %s", suffix)
	}
}

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single call",
			input:    `{"tool_call": {"name": "get_weather", "arguments": {"location": "Дерибасовская"}}}`,
			expected: 1,
		},
		{
			name: "multiple calls",
			input: `{"tool_call": {"name": "get_weather", "arguments": {"location": "Дерибасовская"}}}
{"tool_call": {"name": "get_weather", "arguments": {"location": "Moscow"}}}`,
			expected: 2,
		},
		{
			name:     "no tool calls",
			input:    "Hello, how can I help you?",
			expected: 0,
		},
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := ParseToolCalls(tt.input)
			if len(calls) != tt.expected {
				t.Errorf("expected %d calls, got %d: %+v", tt.expected, len(calls), calls)
			}
		})
	}
}

func TestParseToolCallArguments(t *testing.T) {
	input := `{"tool_call": {"name": "get_weather", "arguments": {"location": "Дерибасовская"}}}`
	calls := ParseToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "get_weather" {
		t.Errorf("name = %q, want %q", calls[0].Name, "get_weather")
	}
	if loc, ok := calls[0].Arguments["location"].(string); !ok || loc != "Дерибасовская" {
		t.Errorf("location = %v, want 'Дерибасовская'", calls[0].Arguments["location"])
	}
}

func TestIsToolCallResponse(t *testing.T) {
	if !IsToolCallResponse(`{"tool_call": {"name": "x", "arguments": {}}}`) {
		t.Error("should detect tool call")
	}
	if IsToolCallResponse("Just a normal message") {
		t.Error("should not detect tool call in normal text")
	}
}

func TestExtractNonToolContent(t *testing.T) {
	input := "Let me check the weather.\n" +
		`{"tool_call": {"name": "get_weather", "arguments": {"location": "x"}}}` + "\n" +
		"And some more text."

	result := ExtractNonToolContent(input)
	if strings.Contains(result, "tool_call") {
		t.Errorf("tool_call should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Let me check") {
		t.Errorf("non-tool content should remain: %s", result)
	}
}

func TestExecuteCalls(t *testing.T) {
	r := NewRegistry()
	RegisterWeather(r)

	calls := []ToolCall{
		{ID: "1", Name: "get_weather", Arguments: map[string]interface{}{"location": "Дерибасовская"}},
	}
	results := r.ExecuteCalls(calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
	if results[0].Content != "На Дерибасовская хорошая погода" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}
}

func TestFormatToolResult(t *testing.T) {
	call := ToolCall{Name: "get_weather"}
	result := FormatToolResult(call, "sunny", nil)
	if result != "sunny" {
		t.Errorf("unexpected result: %q", result)
	}
	result = FormatToolResult(call, "", fmt.Errorf("oops"))
	if !strings.Contains(result, "oops") {
		t.Errorf("error should be in result: %q", result)
	}
}
