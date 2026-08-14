package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPgUpKeyScrolls(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.entries = []entry{
		{role: "assistant", content: strings.Repeat("line\n", 60)},
	}
	m.refreshViewport()

	before := m.viewport.YOffset
	if before == 0 {
		t.Fatalf("expected to be scrolled to bottom, offset=%d", before)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.viewport.YOffset >= before {
		t.Fatalf("pgup did not scroll up: offset %d -> %d", before, m.viewport.YOffset)
	}
	if m.following {
		t.Fatalf("following should be false after pgup")
	}
	if hint := m.scrollHint(); hint == "" {
		t.Fatalf("expected scroll hint after pgup")
	}
}
