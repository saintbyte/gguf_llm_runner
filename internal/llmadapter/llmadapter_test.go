package llmadapter

import "testing"

func TestLoadRejectsNil(t *testing.T) {
	if _, err := Load(nil, nil, "x.gguf", 1.0); err == nil {
		t.Fatal("expected error for nil model/ctx")
	}
}

func TestCloseNilSafe(t *testing.T) {
	var a *Adapter
	a.Close() // не должно паниковать
}
