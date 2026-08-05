package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"interface-load-test/internal/accountpool"
)

func TestModuleRegistryRegisterAndGet(t *testing.T) {
	registry := NewModuleRegistry()
	module := fakeModule{moduleType: "load_test"}

	registry.Register(module)

	got, ok := registry.Get("load_test")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Type() != module.Type() {
		t.Fatalf("Get() module type = %q, want %q", got.Type(), module.Type())
	}
}

func TestModuleRegistryGetMissing(t *testing.T) {
	registry := NewModuleRegistry()

	if module, ok := registry.Get("missing"); ok || module != nil {
		t.Fatalf("Get() = (%v, %v), want (nil, false)", module, ok)
	}
}

func TestModuleRegistryConcurrentRegisterAndGet(t *testing.T) {
	registry := NewModuleRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			registry.Register(fakeModule{moduleType: fmt.Sprintf("module-%d", i)})
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = registry.Get(fmt.Sprintf("module-%d", i))
		}(i)
	}

	wg.Wait()
}

type fakeModule struct {
	moduleType string
}

func (m fakeModule) Type() string {
	return m.moduleType
}

func (m fakeModule) ValidateConfig(json.RawMessage) error {
	return nil
}

func (m fakeModule) ExecutionCount(json.RawMessage) (int, error) {
	return 1, nil
}

func (m fakeModule) Run(context.Context, *Task, []*accountpool.Account) (RunResult, error) {
	return RunResult{}, nil
}
