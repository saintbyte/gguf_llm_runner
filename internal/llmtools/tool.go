package llmtools

import "encoding/json"

// Tool описывает инструмент, доступный модели.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{} // JSON Schema
}

// ToolCall — результат распознавания вызова инструмента из ответа модели.
type ToolCall struct {
	ID        string // уникальный ID вызова (генерируется при парсинге)
	Name      string
	Arguments map[string]interface{}
}

// ToolResult — результат выполнения инструмента.
type ToolResult struct {
	CallID  string
	Content string
	Error   error
}

// Registry хранит зарегистрированные инструменты и выполняет их.
type Registry struct {
	tools map[string]Tool
	funcs map[string]func(args map[string]interface{}) (string, error)
}

// NewRegistry создаёт пустой реестр инструментов.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
		funcs: make(map[string]func(args map[string]interface{}) (string, error)),
	}
}

// Register добавляет инструмент в реестр.
func (r *Registry) Register(t Tool, fn func(args map[string]interface{}) (string, error)) {
	r.tools[t.Name] = t
	r.funcs[t.Name] = fn
}

// Invoke вызывает инструмент по имени с аргументами.
func (r *Registry) Invoke(name string, args map[string]interface{}) (string, error) {
	fn, ok := r.funcs[name]
	if !ok {
		return "", ErrToolNotFound{name}
	}
	return fn(args)
}

// Tools возвращает список всех зарегистрированных инструментов.
func (r *Registry) Tools() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Has проверяет, зарегистрирован ли инструмент.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// SystemPromptSuffix генерирует часть системного промпта с описанием инструментов.
func (r *Registry) SystemPromptSuffix() string {
	if len(r.tools) == 0 {
		return ""
	}

	type paramDef struct {
		Type        string      `json:"type"`
		Description string      `json:"description,omitempty"`
		Enum        interface{} `json:"enum,omitempty"`
	}

	type funcDef struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Parameters  map[string]paramDef `json:"parameters"`
	}

	type toolDef struct {
		Type     string   `json:"type"`
		Function funcDef  `json:"function"`
	}

	defs := make([]toolDef, 0, len(r.tools))
	for _, t := range r.tools {
		params := make(map[string]paramDef)
		for k, v := range t.Parameters {
			p := paramDef{}
			if m, ok := v.(map[string]interface{}); ok {
				if tp, ok := m["type"].(string); ok {
					p.Type = tp
				}
				if d, ok := m["description"].(string); ok {
					p.Description = d
				}
				if e, ok := m["enum"]; ok {
					p.Enum = e
				}
			}
			params[k] = p
		}
		defs = append(defs, toolDef{
			Type: "function",
			Function: funcDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	b, _ := json.MarshalIndent(defs, "", "  ")

	return `
You have access to the following tools. When you need to use a tool, respond ONLY with a JSON tool call, no other text:

` + string(b) + `

To call a tool, output EXACTLY one of these formats (no markdown, no extra text):
- Single call: {"tool_call": {"name": "function_name", "arguments": {"param": "value"}}}
- Multiple calls (one per line):
{"tool_call": {"name": "func1", "arguments": {"a": "b"}}}
{"tool_call": {"name": "func2", "arguments": {"c": "d"}}}

After receiving tool results, use them to answer the user's question naturally.`
}
