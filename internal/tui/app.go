package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	llama "github.com/tcpipuk/llama-go"

	"github.com/saintbyte/gguf_llm_runner/internal/llmcli"
)

type status int

const (
	statusIdle status = iota
	statusGenerating
)

// entry — сообщение в переписке для отображения.
type entry struct {
	role      string // "user" | "assistant" | "system"
	content   string
	thinking  string
	streaming bool
	stats     string // статистика генерации ответа (если есть)
}

// deltaMsg — очередной фрагмент ответа от генерации.
type deltaMsg struct {
	content  string
	thinking string
}

// genDoneMsg — завершение генерации.
type genDoneMsg struct {
	err error
}

// Config — параметры запуска TUI.
type Config struct {
	SystemPrompt string
	MaxTokens    int
	Temperature  float32
	TopP         float32
	TopK         int
	Seed         int
	Timeout      time.Duration // 0 = без таймаута
	ShowThink    bool
	ContextSize  int     // размер контекста (для шапки)
	Threads      int     // потоков CPU (для шапки)
	GPULayers    int     // слоёв на GPU (для шапки; -1 = все)
	LoraPath     string  // путь к LoRA-адаптеру (для шапки)
	LoraScale    float32 // масштаб LoRA
}

// Model — Bubble Tea модель чата.
type Model struct {
	model *llama.Model
	ctx   *llama.Context
	cfg   Config

	messages []llama.ChatMessage
	entries  []entry
	splitter llmcli.ThinkSplitter

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	status    status
	genCancel context.CancelFunc
	cancelled bool
	deltaCh   chan tea.Msg

	// following — авто-прокрутка вниз при стриминге. Выключается, когда
	// пользователь прокрутил вверх (например, читая рассуждения).
	following bool

	// Метрики текущей/последней генерации.
	genStarted time.Time
	genTokens  int

	modelMeta *llama.ModelStats // метаданные модели из GGUF

	userStyle  lipgloss.Style
	asstStyle  lipgloss.Style
	thinkStyle lipgloss.Style
	sysStyle   lipgloss.Style
	statsStyle lipgloss.Style
	userLabel  lipgloss.Style
	asstLabel  lipgloss.Style
	keyStyle   lipgloss.Style
	userBubble lipgloss.Style
	hdr        headerStyles

	width  int
	height int
}

// headerStyles — стили шапки с информацией о модели и параметрах.
type headerStyles struct {
	bar  lipgloss.Style // фоновая плашка
	name lipgloss.Style // имя модели
	dim  lipgloss.Style // остальная часть строки
}

// New создаёт TUI-модель.
func New(model *llama.Model, ctx *llama.Context, cfg Config) *Model {
	ta := textarea.New()
	ta.Placeholder = "Введите сообщение..."
	ta.Prompt = "> "
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(80, 20)
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))))

	hdrBG := lipgloss.Color("236")
	hl := lipgloss.Color("51")
	m := &Model{
		model: model,
		ctx:   ctx,
		cfg:   cfg,

		messages: []llama.ChatMessage{{Role: "system", Content: cfg.SystemPrompt}},
		textarea: ta,
		viewport: vp,
		spinner:  sp,
		deltaCh:  make(chan tea.Msg, 256),
		following: true,

		userStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
		asstStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("120")),
		thinkStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true),
		sysStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
		statsStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		userLabel:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")),
		asstLabel:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("120")),
		keyStyle:   lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("252")).Padding(0, 1),
		userBubble: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("75")).Padding(0, 1),
		hdr: headerStyles{
			bar:  lipgloss.NewStyle().Background(hdrBG).Foreground(lipgloss.Color("250")),
			name: lipgloss.NewStyle().Bold(true).Foreground(hl).Background(hdrBG),
			dim:  lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(hdrBG),
		},
	}

	if model != nil {
		if stats, err := model.Stats(); err == nil {
			m.modelMeta = stats
		}
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	m.textarea.Focus()
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.status == statusGenerating {
				m.cancelled = true
				if m.genCancel != nil {
					m.genCancel()
				}
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			if m.status == statusIdle {
				return m, m.submit()
			}
			return m, nil

		case "tab":
			if m.textarea.Focused() {
				m.textarea.Blur()
			} else {
				m.textarea.Focus()
			}
			return m, nil

		case "pgup", "pgdown":
			// Прокрутка работает и при фокусе на поле ввода.
			m.viewport, cmd = m.viewport.Update(msg)
			m.updateFollowing()
			return m, cmd
		}

		if m.textarea.Focused() {
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}
		m.viewport, cmd = m.viewport.Update(msg)
		m.updateFollowing()
		return m, cmd

	case deltaMsg:
		m.appendDelta(msg)
		return m, m.listenCmd()

	case genDoneMsg:
		m.finalize(msg.err)
		return m, nil

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.updateFollowing()
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	if m.status == statusGenerating {
		cmds = append(cmds, m.spinner.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Загрузка..."
	}
	doc := lipgloss.NewStyle().Padding(0, 1)
	return doc.Render(strings.Join([]string{
		m.header(),
		m.viewport.View(),
		m.textarea.View(),
		m.footer(),
	}, "\n"))
}

// header — шапка с моделью, параметрами генерации и статистикой.
func (m *Model) header() string {
	name, size, arch, gpu := "модель", "", "", ""
	if m.modelMeta != nil {
		md := &m.modelMeta.Metadata
		if md.Name != "" {
			name = md.Name
		}
		size, arch = md.SizeLabel, md.Architecture
		if len(m.modelMeta.GPUs) > 0 {
			names := make([]string, 0, len(m.modelMeta.GPUs))
			for _, g := range m.modelMeta.GPUs {
				names = append(names, fmt.Sprintf("%s (%d МБ)", g.DeviceName, g.TotalMemoryMB))
			}
			gpu = strings.Join(names, ", ")
		}
	}
	info := name
	if size != "" {
		info += " · " + size
	}
	if arch != "" {
		info += " · " + arch
	}
	if gpu != "" {
		info += " · GPU: " + gpu
	}

	params := fmt.Sprintf("ctx %d · max %d · t %.2f · p %.2f · k %d · seed %d · CPU %d",
		m.cfg.ContextSize, m.cfg.MaxTokens, m.cfg.Temperature, m.cfg.TopP, m.cfg.TopK, m.cfg.Seed, m.cfg.Threads)
	if m.cfg.GPULayers >= 0 {
		params += fmt.Sprintf(" · GPU %d слоёв", m.cfg.GPULayers)
	}
	if m.cfg.LoraPath != "" {
		params += fmt.Sprintf(" · lora %s (%.2f)", baseName(m.cfg.LoraPath), m.cfg.LoraScale)
	}

	w := m.width - 2
	if w < 20 {
		w = 20
	}
	bar := m.hdr.bar.Width(w)
	line1 := m.hdr.name.Render(truncate(info, w))
	line2 := m.hdr.dim.Render(truncate(params, w))
	return bar.Render(line1) + "\n" + bar.Render(line2)
}

// headerHeight — число строк, занимаемое шапкой.
func (m *Model) headerHeight() int {
	return strings.Count(m.header(), "\n") + 1
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func baseName(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func (m *Model) footer() string {
	switch m.status {
	case statusGenerating:
		s := m.spinner.View() + " генерация... (Ctrl-C — прервать)"
		if hint := m.scrollHint(); hint != "" {
			s += " | " + hint
		}
		return s
	default:
		s := m.key("Enter") + " отправить | " + m.key("Tab") + " прокрутка | " +
			m.key("/clear") + " сброс | " + m.key("/help") + " | " +
			m.key("Ctrl-C") + " выход"
		if hint := m.scrollHint(); hint != "" {
			s += " | " + hint
		}
		return s
	}
}

func (m *Model) key(k string) string {
	return m.keyStyle.Render(k)
}

// scrollHint — подсказка о скрытом содержимом, когда пользователь отлистал вверх.
func (m *Model) scrollHint() string {
	if m.viewport.TotalLineCount() <= m.viewport.Height {
		return ""
	}
	switch {
	case !m.viewport.AtTop() && !m.viewport.AtBottom():
		return "↑/↓ прокрутка"
	case !m.viewport.AtBottom():
		return "↓ вниз"
	default:
		return ""
	}
}

func (m *Model) layout() {
	if m.width < 1 || m.height < 1 {
		return
	}
	inputHeight := 3
	vpH := m.height - m.headerHeight() - inputHeight - 1 // 1 = footer
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.Width = m.width - 2
	m.viewport.Height = vpH
	m.textarea.SetWidth(m.width - 2)
	m.textarea.SetHeight(inputHeight)
	m.refreshViewport()
}

func (m *Model) submit() tea.Cmd {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return nil
	}
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	m.textarea.Reset()
	m.entries = append(m.entries, entry{role: "user", content: input})
	m.messages = append(m.messages, llama.ChatMessage{Role: "user", Content: input})

	m.splitter = llmcli.ThinkSplitter{}
	m.cancelled = false
	m.genStarted = time.Now()
	m.genTokens = 0
	m.entries = append(m.entries, entry{role: "assistant", streaming: true})
	m.status = statusGenerating
	m.refreshViewport()

	return tea.Batch(m.startGeneration(), m.spinner.Tick)
}

func (m *Model) handleCommand(input string) tea.Cmd {
	m.textarea.Reset()
	switch strings.ToLower(input) {
	case "/exit", "/quit":
		return tea.Quit

	case "/clear":
		m.entries = nil
		m.messages = []llama.ChatMessage{{Role: "system", Content: m.systemPrompt()}}
		m.refreshViewport()

	case "/help":
		m.entries = append(m.entries, entry{
			role:    "system",
			content: "/clear — сбросить диалог\n/exit, /quit — выход\n/help — эта справка\n\nEnter — отправить, Tab — переключить фокус (прокрутка), Ctrl-C — прервать/выйти",
		})
		m.refreshViewport()

	default:
		m.entries = append(m.entries, entry{role: "system", content: "Неизвестная команда: " + input})
		m.refreshViewport()
	}
	return nil
}

func (m *Model) systemPrompt() string {
	if len(m.messages) > 0 && m.messages[0].Role == "system" {
		return m.messages[0].Content
	}
	return ""
}

func (m *Model) startGeneration() tea.Cmd {
	var genCtx context.Context
	var cancel context.CancelFunc
	if m.cfg.Timeout > 0 {
		genCtx, cancel = context.WithTimeout(context.Background(), m.cfg.Timeout)
	} else {
		genCtx, cancel = context.WithCancel(context.Background())
	}
	m.genCancel = cancel

	opts := llama.ChatOptions{
		MaxTokens:   llama.Int(m.cfg.MaxTokens),
		Temperature: llama.Float32(m.cfg.Temperature),
		TopP:        llama.Float32(m.cfg.TopP),
		TopK:        llama.Int(m.cfg.TopK),
		Seed:        llama.Int(m.cfg.Seed),
	}

	deltaCh, errCh := m.ctx.ChatStream(genCtx, m.messages, opts)

	go func() {
		for {
			select {
			case d, ok := <-deltaCh:
				if !ok {
					var err error
					select {
					case err = <-errCh:
					default:
					}
					m.deltaCh <- genDoneMsg{err: err}
					return
				}
				m.deltaCh <- deltaMsg{content: d.Content, thinking: d.ReasoningContent}
			case err := <-errCh:
				m.deltaCh <- genDoneMsg{err: err}
				return
			case <-genCtx.Done():
				m.deltaCh <- genDoneMsg{err: genCtx.Err()}
				return
			}
		}
	}()

	return m.listenCmd()
}

func (m *Model) listenCmd() tea.Cmd {
	return func() tea.Msg {
		return <-m.deltaCh
	}
}

func (m *Model) appendDelta(d deltaMsg) {
	idx := len(m.entries) - 1
	if idx < 0 {
		return
	}
	m.genTokens++
	if d.thinking != "" {
		m.entries[idx].thinking += d.thinking
	}
	if d.content != "" {
		for _, seg := range m.splitter.Feed(d.content) {
			switch seg.Kind {
			case llmcli.SegmentContent:
				m.entries[idx].content += seg.Text
			case llmcli.SegmentThinking:
				m.entries[idx].thinking += seg.Text
			}
		}
	}
	m.refreshViewport()
}

func (m *Model) finalize(err error) {
	if m.genCancel != nil {
		m.genCancel()
		m.genCancel = nil
	}
	idx := len(m.entries) - 1
	if idx >= 0 {
		for _, seg := range m.splitter.Flush() {
			switch seg.Kind {
			case llmcli.SegmentContent:
				m.entries[idx].content += seg.Text
			case llmcli.SegmentThinking:
				m.entries[idx].thinking += seg.Text
			}
		}
		m.entries[idx].streaming = false
		m.entries[idx].stats = m.runStats()
	}

	if err != nil {
		msg := fmt.Sprintf("Ошибка генерации: %v", err)
		if m.cancelled {
			msg = "(генерация прервана)"
		}
		m.entries = append(m.entries, entry{role: "system", content: msg})
	} else if !m.cancelled && idx >= 0 {
		m.messages = append(m.messages, llama.ChatMessage{Role: "assistant", Content: m.entries[idx].content})
	}

	m.cancelled = false
	m.status = statusIdle
	m.refreshViewport()
}

// runStats — статистика последней генерации: токены, скорость, время, контекст.
func (m *Model) runStats() string {
	elapsed := time.Since(m.genStarted)
	if m.genTokens <= 0 || elapsed <= 0 {
		return ""
	}
	tps := float64(m.genTokens) / elapsed.Seconds()
	s := fmt.Sprintf("· %d токенов · %.1f ток/с · %s", m.genTokens, tps, elapsed.Round(time.Second))
	if m.ctx != nil {
		if total, err := m.ctx.GetCachedTokenCount(); err == nil && total >= m.genTokens {
			s += fmt.Sprintf(" · контекст %d токенов", total)
		}
	}
	return s
}

func (m *Model) renderContent() string {
	var b strings.Builder
	for i := range m.entries {
		e := &m.entries[i]
		switch e.role {
		case "user":
			b.WriteString("\n")
			b.WriteString(m.userLabel.Render("You"))
			b.WriteString("\n")
			b.WriteString(m.userBubble.Render(e.content))
			b.WriteString("\n\n")

		case "assistant":
			b.WriteString("\n")
			if e.streaming {
				b.WriteString(m.asstLabel.Render("Assistant "))
				b.WriteString(m.spinner.View())
			} else {
				b.WriteString(m.asstLabel.Render("Assistant"))
			}
			b.WriteString("\n")
			if m.cfg.ShowThink && e.thinking != "" {
				b.WriteString(m.thinkStyle.Render(strings.TrimSpace(e.thinking)))
				b.WriteString("\n\n")
			}
			b.WriteString(m.asstStyle.Render(e.content))
			if e.stats != "" {
				b.WriteString("\n" + m.statsStyle.Render(e.stats))
			}
			b.WriteString("\n\n")

		case "system":
			b.WriteString("\n")
			b.WriteString(m.sysStyle.Render(e.content))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(m.renderContent())
	if m.following {
		m.viewport.GotoBottom()
	}
}

// updateFollowing синхронизирует авто-прокрутку с позицией пользователя:
// уехал вверх — останавливаем, вернулся вниз — снова следим.
func (m *Model) updateFollowing() {
	m.following = m.viewport.AtBottom()
}
