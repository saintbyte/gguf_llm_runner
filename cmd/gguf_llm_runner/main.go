package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	llama "github.com/tcpipuk/llama-go"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/saintbyte/gguf_llm_runner/internal/llmadapter"
	"github.com/saintbyte/gguf_llm_runner/internal/llmcli"
	"github.com/saintbyte/gguf_llm_runner/internal/tui"
)

var (
	modelPath   = flag.String("model", "models/Qwen3-0.6B-Q8_0.gguf", "путь к GGUF-модели")
	system      = flag.String("system", "You are a helpful assistant.", "системный промпт")
	message     = flag.String("message", "", "однократный запрос (пусто = интерактивный режим)")
	contextSize = flag.Int("context", 4096, "размер контекста (0 = нативный максимум модели)")
	maxTokens   = flag.Int("max-tokens", 1024, "максимум токенов в ответе")
	temp        = flag.Float64("temperature", 0.7, "temperature (0.0-2.0)")
	topP        = flag.Float64("top-p", 0.9, "nucleus sampling")
	topK        = flag.Int("top-k", 40, "top-K sampling")
	seed        = flag.Int("seed", -1, "сид генерации (-1 = случайный)")
	gpuLayers   = flag.Int("ngl", -1, "слоёв на GPU (-1 = все)")
	threads     = flag.Int("threads", runtime.NumCPU(), "число потоков CPU")
	timeout     = flag.Int("timeout", 120, "таймаут генерации в секундах")
	reasoning   = flag.Bool("reasoning", true, "выводить рассуждения (для reasoning-моделей)")
	streaming   = flag.Bool("stream", true, "стриминг ответа")
	useTUI      = flag.Bool("tui", true, "интерактивный режим через TUI (иначе обычный ввод)")
	loraPath    = flag.String("lora", "", "путь к LoRA-адаптеру (.gguf), применяется к контексту")
	loraScale   = flag.Float64("lora-scale", 1.0, "масштаб LoRA-адаптера")
	debug       = flag.Bool("debug", false, "показывать историю и отладочный вывод llama.cpp")
)

func main() {
	flag.Parse()

	if *debug {
		os.Setenv("LLAMA_LOG", "info")
	} else {
		os.Setenv("LLAMA_LOG", "error")
	}
	llama.InitLogging()

	fmt.Printf("Загрузка модели: %s\n", *modelPath)
	model, err := llama.LoadModel(*modelPath, llama.WithGPULayers(*gpuLayers))
	if err != nil {
		log.Fatalf("Не удалось загрузить модель: %v", err)
	}
	defer model.Close()

	contextOpts := []llama.ContextOption{
		llama.WithThreads(*threads),
	}
	if *contextSize != 0 {
		contextOpts = append(contextOpts, llama.WithContext(*contextSize))
	}

	ctx, err := model.NewContext(contextOpts...)
	if err != nil {
		log.Fatalf("Не удалось создать контекст: %v", err)
	}
	defer ctx.Close()

	if *loraPath != "" {
		adapter, err := llmadapter.Load(model, ctx, *loraPath, float32(*loraScale))
		if err != nil {
			log.Fatalf("LoRA: %v", err)
		}
		defer adapter.Close()
		fmt.Printf("LoRA: %s (scale %.2f)\n", *loraPath, *loraScale)
	}

	printModelStats(model)

	interruptCh := make(chan os.Signal, 1)
	signal.Notify(interruptCh, os.Interrupt, syscall.SIGTERM)

	if *message != "" {
		runSingleMessage(model, ctx, interruptCh)
	} else if *useTUI && term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd())) {
		runTUI(model, ctx)
	} else {
		runPlainInteractive(model, ctx, interruptCh)
	}
}

func chatOptions() llama.ChatOptions {
	opts := llama.ChatOptions{
		MaxTokens:   llama.Int(*maxTokens),
		Temperature: llama.Float32(float32(*temp)),
		TopP:        llama.Float32(float32(*topP)),
		TopK:        llama.Int(*topK),
		Seed:        llama.Int(*seed),
	}
	return opts
}

func runSingleMessage(model *llama.Model, ctx *llama.Context, interruptCh <-chan os.Signal) {
	messages := []llama.ChatMessage{
		{Role: "system", Content: *system},
		{Role: "user", Content: *message},
	}
	llmcli.PrintSystemPrompt(*system)
	llmcli.PrintUserInput(*message)

	respond(ctx, messages, interruptCh)
}

func runTUI(model *llama.Model, ctx *llama.Context) {
	cfg := tui.Config{
		SystemPrompt: *system,
		MaxTokens:    *maxTokens,
		Temperature:  float32(*temp),
		TopP:         float32(*topP),
		TopK:         *topK,
		Seed:         *seed,
		Timeout:      time.Duration(*timeout) * time.Second,
		ShowThink:    *reasoning,
		ContextSize:  *contextSize,
		Threads:      *threads,
		GPULayers:    *gpuLayers,
		LoraPath:     *loraPath,
		LoraScale:    float32(*loraScale),
	}
	p := tea.NewProgram(tui.New(model, ctx, cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Ошибка TUI: %v", err)
	}
	fmt.Println()
}

func runPlainInteractive(model *llama.Model, ctx *llama.Context, interruptCh <-chan os.Signal) {
	llmcli.PrintBanner("Интерактивный режим")
	llmcli.PrintSystemPrompt(*system)
	fmt.Printf("Параметры: max_tokens=%d, temperature=%.2f, top_p=%.2f, top_k=%d, ngl=%d, threads=%d",
		*maxTokens, *temp, *topP, *topK, *gpuLayers, *threads)
	if *loraPath != "" {
		fmt.Printf(", lora=%s (scale %.2f)", *loraPath, *loraScale)
	}
	fmt.Println()
	fmt.Println("Введите сообщение. Пустая строка, 'exit'/'quit' или Ctrl-C — выход.")

	messages := []llama.ChatMessage{
		{Role: "system", Content: *system},
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("You > ")
		if !scanner.Scan() {
			fmt.Println()
			break
		}
		input := strings.TrimSpace(scanner.Text())
		switch {
		case input == "":
			continue
		case isExitCommand(input):
			fmt.Println("Пока!")
			return
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Ошибка чтения ввода: %v", err)
			return
		}

		messages = append(messages, llama.ChatMessage{Role: "user", Content: input})

		if *debug {
			formatted, err := model.FormatChatPrompt(messages, chatOptions())
			fmt.Printf("\n%s[Debug] Отформатированный промпт:%s\n%s\n", "\033[2m", "\033[0m", formatted)
			if err != nil {
				log.Printf("[Debug] Ошибка форматирования промпта: %v", err)
			}
		}

		content := respond(ctx, messages, interruptCh)
		messages = append(messages, llama.ChatMessage{Role: "assistant", Content: content})
	}
}

func isExitCommand(s string) bool {
	switch strings.ToLower(s) {
	case "exit", "quit", "/exit", "/quit", "bye", "/bye":
		return true
	}
	return false
}

// respond генерирует ответ и возвращает контент для истории диалога.
func respond(ctx *llama.Context, messages []llama.ChatMessage, interruptCh <-chan os.Signal) string {
	genCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *timeout > 0 {
		var c context.CancelFunc
		genCtx, c = context.WithTimeout(genCtx, time.Duration(*timeout)*time.Second)
		defer c()
	}

	renderer := llmcli.NewStreamRenderer(os.Stdout)
	renderer.WithExtraction(*reasoning, *reasoning)
	llmcli.PrintAssistantHeader()

	if *streaming {
		return streamResponse(genCtx, ctx, messages, renderer, interruptCh, cancel)
	}

	response, err := ctx.Chat(genCtx, messages, chatOptions())
	if err != nil {
		renderer.HandleError(err)
		return ""
	}
	renderer.ProcessDelta(llama.ChatDelta{Content: response.Content})
	return renderer.Finish()
}

func streamResponse(genCtx context.Context, ctx *llama.Context, messages []llama.ChatMessage, renderer *llmcli.StreamRenderer, interruptCh <-chan os.Signal, cancel context.CancelFunc) string {
	deltaCh, errCh := ctx.ChatStream(genCtx, messages, chatOptions())
	for {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				return renderer.Finish()
			}
			renderer.ProcessDelta(delta)

		case err := <-errCh:
			if err != nil {
				return renderer.HandleError(err)
			}

		case <-genCtx.Done():
			return renderer.HandleTimeout(genCtx.Err())

		case <-interruptCh:
			cancel()
			fmt.Printf("\n%s(генерация прервана)%s\n", "\033[2m", "\033[0m")
			return renderer.Finish()
		}
	}
}

func printModelStats(model *llama.Model) {
	stats, err := model.Stats()
	if err != nil {
		log.Printf("Не удалось получить статистику модели: %v", err)
		return
	}
	m := stats.Metadata
	if m.Name != "" {
		fmt.Printf("Модель: %s", m.Name)
		if m.SizeLabel != "" {
			fmt.Printf(" (%s)", m.SizeLabel)
		}
		fmt.Println()
	}
	if m.Architecture != "" {
		fmt.Printf("Архитектура: %s\n", m.Architecture)
	}
	fmt.Println()
}
