package task

import "sync"

// ModuleRegistry stores business modules by type.
type ModuleRegistry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

// NewModuleRegistry creates an empty registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]Module),
	}
}

// Register stores or replaces a module by its Type.
func (r *ModuleRegistry) Register(m Module) {
	if m == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules[m.Type()] = m
}

// Get returns the module registered for moduleType.
func (r *ModuleRegistry) Get(moduleType string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.modules[moduleType]
	return m, ok
}
