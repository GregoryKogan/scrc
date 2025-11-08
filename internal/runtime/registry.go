package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"scrc/internal/domain/execution"
)

// Registry wires language modules into a single Engine implementation.
type Registry struct {
	mu      sync.RWMutex
	modules map[execution.Language]Module
}

// NewRegistry constructs a registry from the supplied modules.
func NewRegistry(mods ...Module) (*Registry, error) {
	reg := &Registry{
		modules: make(map[execution.Language]Module, len(mods)),
	}

	for _, module := range mods {
		if module == nil {
			return nil, fmt.Errorf("runtime module cannot be nil")
		}

		lang := module.Language()
		if lang == "" {
			return nil, fmt.Errorf("runtime module missing language identifier")
		}
		if _, exists := reg.modules[lang]; exists {
			return nil, fmt.Errorf("duplicate runtime module for language %q", lang)
		}

		reg.modules[lang] = module
	}

	if len(reg.modules) == 0 {
		return nil, fmt.Errorf("at least one runtime module must be registered")
	}

	return reg, nil
}

// Prepare dispatches the request to the module responsible for the script's language.
func (r *Registry) Prepare(ctx context.Context, script execution.Script) (PreparedScript, *execution.Result, error) {
	module, err := r.moduleFor(script.Language)
	if err != nil {
		return nil, nil, err
	}
	return module.Prepare(ctx, script)
}

// Close releases resources held by each module.
func (r *Registry) Close() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for lang, module := range r.modules {
		if err := module.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lang, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (r *Registry) moduleFor(lang execution.Language) (Module, error) {
	r.mu.RLock()
	module, ok := r.modules[lang]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no runtime module registered for language %q", lang)
	}
	return module, nil
}
