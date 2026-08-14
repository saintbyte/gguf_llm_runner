// Package llmadapter загружает и применяет LoRA-адаптеры к контексту
// llama-go через прямой вызов C API llama.cpp.
package llmadapter

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/llama-go/llama.cpp/include -I${SRCDIR}/../../third_party/llama-go/llama.cpp/ggml/include -I${SRCDIR}/../../third_party/llama-go/llama.cpp/common
#include <stdlib.h>
#include <llama.h>

// llama-go хранит в Model.modelPtr / Context.contextPtr указатели на свои
// wrapper-структуры, у которых первым полем идёт сырой указатель на
// llama_model* / llama_context*. Извлекаем его напрямую.
static void* wrapper_inner(void* wrapper) {
	return *(void**)wrapper;
}

static void* load_lora(void* wrapper_model, const char* path) {
	return llama_adapter_lora_init((struct llama_model*)wrapper_inner(wrapper_model), path);
}

static int apply_lora(void* wrapper_ctx, void** adapters, int n, float* scales) {
	return llama_set_adapters_lora((struct llama_context*)wrapper_inner(wrapper_ctx),
		(struct llama_adapter_lora**)adapters, (size_t)n, scales);
}

static void free_lora(void* adapter) {
	llama_adapter_lora_free((struct llama_adapter_lora*)adapter);
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	llama "github.com/tcpipuk/llama-go"
)

// modelFields и contextFields зеркалят первые поля внутренних структур
// llama-go (Model и Context), чтобы получить сырые указатели.
type modelFields struct {
	modelPtr unsafe.Pointer
}

type contextFields struct {
	contextPtr unsafe.Pointer
}

func modelInner(m *llama.Model) unsafe.Pointer {
	return (*modelFields)(unsafe.Pointer(m)).modelPtr
}

func ctxInner(c *llama.Context) unsafe.Pointer {
	return (*contextFields)(unsafe.Pointer(c)).contextPtr
}

// Adapter — загруженный LoRA-адаптер, применённый к контексту.
type Adapter struct {
	ptr   unsafe.Pointer
	Path  string
	Scale float32
}

// Load загружает LoRA-адаптер из файла и применяет его к контексту.
//
// Адаптер живёт столько же, сколько и модель: llama_model освобождает
// связанные адаптеры при закрытии. Close() можно вызвать и раньше.
func Load(model *llama.Model, ctx *llama.Context, path string, scale float32) (*Adapter, error) {
	if model == nil || ctx == nil {
		return nil, fmt.Errorf("model и ctx не должны быть nil")
	}

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ptr := C.load_lora(modelInner(model), cPath)
	if ptr == nil {
		return nil, fmt.Errorf("не удалось загрузить LoRA-адаптер %q: проверьте путь и совместимость с моделью", path)
	}

	a := &Adapter{ptr: ptr, Path: path, Scale: scale}

	var adapters = [1]unsafe.Pointer{a.ptr}
	var scales = [1]C.float{C.float(scale)}
	if rc := C.apply_lora(ctxInner(ctx), &adapters[0], 1, &scales[0]); rc != 0 {
		a.Close()
		return nil, fmt.Errorf("не удалось применить LoRA-адаптер %q (код %d)", path, int(rc))
	}
	return a, nil
}

// Close освобождает адаптер. Не обязателен к вызову — модель сама
// освободит связанные адаптеры при закрытии.
func (a *Adapter) Close() {
	if a != nil && a.ptr != nil {
		C.free_lora(a.ptr)
		a.ptr = nil
	}
}
