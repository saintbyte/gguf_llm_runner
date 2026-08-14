package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	llama "github.com/tcpipuk/llama-go"
)

func newTestModel() *Model {
	return New(nil, nil, Config{
		SystemPrompt: "Test system prompt",
		MaxTokens:    100,
		Temperature:  0.7,
		TopP:         0.9,
		TopK:         40,
		Seed:         -1,
		Timeout:      time.Minute,
		ShowThink:    true,
	})
}

func TestWindowSizeAndRender(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.entries = []entry{
		{role: "user", content: "Hello"},
		{role: "assistant", content: "Hi there", thinking: "Let me think"},
	}
	m.refreshViewport()

	view := m.View()
	for _, want := range []string{"You", "Hello", "Assistant", "Hi there", "Let me think"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q, got:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "отправить") {
		t.Errorf("footer should show hints in idle state")
	}
	if !strings.Contains(view, "max 100") {
		t.Errorf("header should show generation params, got:\n%s", view)
	}
}

func TestDeltaStreamingWithThinking(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.entries = []entry{{role: "user", content: "2+2?"}}
	m.entries = append(m.entries, entry{role: "assistant", streaming: true})

	// Дельты разбиты так, что тег <think> приходит по токенам.
	for _, tok := range []string{"<thi", "nk>", "Let me compute.", "</th", "ink>", "\n\nThe answer is 4."} {
		m.appendDelta(deltaMsg{content: tok})
	}

	last := &m.entries[len(m.entries)-1]
	if got := last.content; got != "\n\nThe answer is 4." {
		t.Errorf("content = %q, want %q", got, "\n\nThe answer is 4.")
	}
	if got := last.thinking; got != "Let me compute." {
		t.Errorf("thinking = %q, want %q", got, "Let me compute.")
	}

	view := m.viewport.View()
	if strings.Contains(view, "<think>") {
		t.Errorf("raw <think> tag leaked into view: %s", view)
	}
	if !strings.Contains(view, "Let me compute.") {
		t.Errorf("thinking should be visible in view")
	}
}

func TestFinalizeAppendsToHistory(t *testing.T) {
	m := newTestModel()
	m.entries = []entry{
		{role: "user", content: "hi"},
		{role: "assistant", content: "hello", streaming: true},
	}
	m.status = statusGenerating

	m.finalize(nil)

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(m.messages))
	}
	if m.messages[1].Role != "assistant" || m.messages[1].Content != "hello" {
		t.Errorf("assistant message not appended to history: %+v", m.messages[1])
	}
	if m.status != statusIdle {
		t.Errorf("status = %v, want idle", m.status)
	}
}

func TestClearCommand(t *testing.T) {
	m := newTestModel()
	m.entries = []entry{{role: "user", content: "x"}}
	m.messages = []llama.ChatMessage{{Role: "user", Content: "x"}}

	cmd := m.handleCommand("/clear")
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if len(m.entries) != 0 {
		t.Errorf("entries not cleared: %d", len(m.entries))
	}
	if len(m.messages) != 1 || m.messages[0].Role != "system" {
		t.Errorf("messages not reset to system: %+v", m.messages)
	}
}

func TestScrollPauseFollow(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	long := strings.Repeat("line\n", 60)
	m.entries = []entry{
		{role: "user", content: "Q"},
		{role: "assistant", content: long, thinking: strings.Repeat("think\n", 40)},
	}
	m.refreshViewport()

	if !m.following {
		t.Errorf("following should be true initially")
	}
	if !m.viewport.AtBottom() {
		t.Errorf("viewport should be at bottom initially")
	}

	m.viewport.ScrollUp(10)
	m.updateFollowing()
	if m.following {
		t.Errorf("following should be false after scrolling up")
	}
	yBefore := m.viewport.YOffset

	m.appendDelta(deltaMsg{content: "more\ncontent\n"})
	if m.following {
		t.Errorf("following should stay false after deltas")
	}
	if m.viewport.YOffset != yBefore {
		t.Errorf("offset changed while not following: %d -> %d", yBefore, m.viewport.YOffset)
	}

	m.viewport.GotoBottom()
	m.updateFollowing()
	if !m.following {
		t.Errorf("following should be true after scrolling to bottom")
	}
	m.appendDelta(deltaMsg{content: "tail\n"})
	if !m.viewport.AtBottom() {
		t.Errorf("viewport should auto-scroll to bottom when following")
	}
	if hint := m.scrollHint(); hint != "" {
		t.Errorf("no scroll hint expected at bottom, got %q", hint)
	}

	m.viewport.ScrollUp(5)
	m.updateFollowing()
	if hint := m.scrollHint(); hint != "↑/↓ прокрутка" {
		t.Errorf("scroll hint mid-position = %q, want %q", hint, "↑/↓ прокрутка")
	}
	if !strings.Contains(m.View(), "прокрутка") {
		t.Errorf("footer should show scroll hint in view")
	}

	m.viewport.GotoTop()
	m.updateFollowing()
	if hint := m.scrollHint(); hint != "↓ вниз" {
		t.Errorf("scroll hint at top = %q, want %q", hint, "↓ вниз")
	}
	if !strings.Contains(m.View(), "↓ вниз") {
		t.Errorf("footer should show '↓ вниз' hint at top")
	}
}

func TestHeaderShowsModelInfo(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	for _, want := range []string{"модель", "ctx 0", "max 100", "seed -1", "CPU 0"} {
		if !strings.Contains(view, want) {
			t.Errorf("header should contain %q, got:\n%s", want, view)
		}
	}
}

func TestHeaderShowsLora(t *testing.T) {
	m := newTestModel()
	m.cfg.LoraPath = "models/my-adapter.gguf"
	m.cfg.LoraScale = 0.8
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := m.View()
	for _, want := range []string{"lora", "my-adapter.gguf", "0.80"} {
		if !strings.Contains(view, want) {
			t.Errorf("header should show lora %q, got:\n%s", want, view)
		}
	}
}

func TestFinalizeSetsStats(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.entries = []entry{
		{role: "assistant", content: "hi", streaming: true},
	}
	m.genStarted = time.Now()
	m.genTokens = 12

	m.finalize(nil)

	if m.entries[0].stats == "" {
		t.Errorf("stats should be set after finalize")
	}
	if !strings.Contains(m.View(), "токенов") {
		t.Errorf("stats should be visible in view")
	}
}
