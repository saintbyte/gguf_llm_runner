package llmtools

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrToolNotFound возвращается при вызове несуществующего инструмента.
type ErrToolNotFound struct {
	Name string
}

func (e ErrToolNotFound) Error() string {
	return fmt.Sprintf("tool not found: %s", e.Name)
}

var toolCallRe = regexp.MustCompile(`\{"tool_call"\s*:\s*\{[^}]*"name"\s*:\s*"([^"]+)"[^}]*"arguments"\s*:\s*(\{[^}]*\})\s*\}\s*\}`)

// ParseToolCalls извлекает вызовы инструментов из текста ответа модели.
// Поддерживает несколько tool call'ов в одном ответе (по одному на строку).
func ParseToolCalls(text string) []ToolCall {
	lines := strings.Split(text, "\n")
	var calls []ToolCall

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tc, ok := parseToolCallLine(line)
		if ok {
			calls = append(calls, tc)
		}
	}
	return calls
}

// IsToolCallResponse проверяет, содержит ли ответ модели tool calls.
func IsToolCallResponse(text string) bool {
	return strings.Contains(text, `"tool_call"`)
}

func parseToolCallLine(line string) (ToolCall, bool) {
	// Попробуем распарсить как JSON
	var wrapper struct {
		ToolCall *struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		} `json:"tool_call"`
	}
	if err := json.Unmarshal([]byte(line), &wrapper); err == nil && wrapper.ToolCall != nil {
		return ToolCall{
			Name:      wrapper.ToolCall.Name,
			Arguments: wrapper.ToolCall.Arguments,
		}, true
	}

	// Fallback: regex для повреждённого JSON
	matches := toolCallRe.FindStringSubmatch(line)
	if len(matches) < 3 {
		return ToolCall{}, false
	}

	name := matches[1]
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(matches[2]), &args); err != nil {
		args = make(map[string]interface{})
	}

	return ToolCall{
		Name:      name,
		Arguments: args,
	}, true
}

// FormatToolResult форматирует результат выполнения инструмента
// как сообщение для добавления в историю диалога.
func FormatToolResult(call ToolCall, result string, err error) string {
	if err != nil {
		return fmt.Sprintf("Error executing tool %s: %v", call.Name, err)
	}
	return result
}

// FormatToolMessage возвращает Role и Content для tool-результата.
func FormatToolMessage(call ToolCall, result string, err error) (role, content string) {
	return "tool", FormatToolResult(call, result, err)
}

// ExtractNonToolContent возвращает текст ответа за вычетом tool call строк.
func ExtractNonToolContent(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `{"tool_call"`) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ExecuteCalls выполняет все tool calls и возвращает результаты.
func (r *Registry) ExecuteCalls(calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, call := range calls {
		content, err := r.Invoke(call.Name, call.Arguments)
		results[i] = ToolResult{
			CallID:  call.ID,
			Content: content,
			Error:   err,
		}
	}
	return results
}

// HasToolCallsInDelta проверяет, может ли накопленный текст содержать начало tool call.
// Возвращает true если текст похож на начало JSON tool call.
func HasToolCallsInDelta(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.Contains(trimmed, `"tool_call"`)
}

// errors.Is compatible check
func isErrToolNotFound(err error) bool {
	var target ErrToolNotFound
	return errors.As(err, &target)
}
