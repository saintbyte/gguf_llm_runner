package llmcli

import (
	"fmt"
	"io"
	"strings"

	llama "github.com/tcpipuk/llama-go"
)

const (
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiReset  = "\033[0m"
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
)

// StreamRenderer выводит токены в реальном времени и накапливает результат.
//
// Для reasoning-моделей (Qwen3, DeepSeek-R1 и т.п.) сегменты между тегами
// <think>...</think> выводятся приглушённым цветом и не попадают в историю
// диалога.
type StreamRenderer struct {
	writer   io.Writer
	splitter ThinkSplitter

	content   strings.Builder // чистый ответ (для истории диалога)
	thinking  strings.Builder // накопленные рассуждения
	extract   bool            // обрабатывать теги рассуждений
	showThink bool            // показывать рассуждения (при extract=true)
}

func NewStreamRenderer(w io.Writer) *StreamRenderer {
	return &StreamRenderer{writer: w, extract: true, showThink: true}
}

// WithExtraction включает/выключает обработку тегов рассуждений.
func (r *StreamRenderer) WithExtraction(enabled, show bool) *StreamRenderer {
	r.extract = enabled
	r.showThink = show
	return r
}

func (r *StreamRenderer) ProcessDelta(d llama.ChatDelta) {
	if d.ReasoningContent != "" {
		r.writeThinking(d.ReasoningContent)
	}
	if d.Content != "" {
		r.feed(d.Content)
	}
}

func (r *StreamRenderer) feed(text string) {
	if !r.extract {
		r.emitContent(text)
		return
	}
	for _, seg := range r.splitter.Feed(text) {
		switch seg.Kind {
		case SegmentContent:
			r.emitContent(seg.Text)
		case SegmentThinking:
			r.emitThinking(seg.Text)
		}
	}
}

func (r *StreamRenderer) emitContent(s string) {
	if s == "" {
		return
	}
	fmt.Fprint(r.writer, s)
	r.content.WriteString(s)
}

func (r *StreamRenderer) emitThinking(s string) {
	if s == "" {
		return
	}
	r.thinking.WriteString(s)
	if r.showThink {
		fmt.Fprintf(r.writer, "%s%s%s", ansiDim, s, ansiReset)
	}
}

func (r *StreamRenderer) writeThinking(s string) {
	r.thinking.WriteString(s)
	if r.showThink {
		fmt.Fprintf(r.writer, "%s%s%s", ansiDim, s, ansiReset)
	}
}

// Finish сбрасывает буфер и возвращает чистый контент ответа (без рассуждений).
func (r *StreamRenderer) Finish() string {
	for _, seg := range r.splitter.Flush() {
		switch seg.Kind {
		case SegmentContent:
			r.emitContent(seg.Text)
		case SegmentThinking:
			r.emitThinking(seg.Text)
		}
	}
	fmt.Fprintln(r.writer)
	return r.content.String()
}

func (r *StreamRenderer) HandleError(err error) string {
	fmt.Fprintf(r.writer, "\n%sОшибка генерации: %v%s\n", ansiYellow, err, ansiReset)
	return r.content.String()
}

func (r *StreamRenderer) HandleTimeout(err error) string {
	fmt.Fprintf(r.writer, "\n%sТаймаут генерации: %v%s\n", ansiYellow, err, ansiReset)
	return r.content.String()
}

func PrintUserInput(msg string) {
	fmt.Printf("%sYou >%s %s\n", ansiCyan, ansiReset, msg)
}

func PrintAssistantHeader() {
	fmt.Printf("%sAssistant >%s ", ansiGreen, ansiReset)
}

func PrintBanner(title string) {
	fmt.Printf("%s=== %s ===%s\n", ansiBold, title, ansiReset)
}

func PrintSystemPrompt(p string) {
	if p == "" {
		return
	}
	fmt.Printf("%sSystem:%s %s\n", ansiDim, ansiReset, p)
}
