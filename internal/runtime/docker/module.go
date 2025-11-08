package docker

import (
	"context"
	"fmt"

	"scrc/internal/domain/execution"
	runtimex "scrc/internal/runtime"
)

type languageStrategy interface {
	Prepare(ctx context.Context, lang *languageRuntime, script execution.Script) (runtimex.PreparedScript, *execution.Result, error)
	Close() error
}

type module struct {
	runtime  *languageRuntime
	strategy languageStrategy
}

func newModule(lang execution.Language, cfg LanguageConfig, engine *containerEngine) (runtimex.Module, error) {
	runtime, err := newLanguageRuntime(lang, cfg, engine)
	if err != nil {
		return nil, err
	}

	strategy, err := strategyForLanguage(lang)
	if err != nil {
		return nil, err
	}

	return &module{
		runtime:  runtime,
		strategy: strategy,
	}, nil
}

func (m *module) Language() execution.Language {
	return m.runtime.language
}

func (m *module) Prepare(ctx context.Context, script execution.Script) (runtimex.PreparedScript, *execution.Result, error) {
	if script.Language != m.runtime.language {
		return nil, nil, fmt.Errorf("docker runtime: script language %q does not match module %q", script.Language, m.runtime.language)
	}

	if err := m.runtime.ensureImage(ctx); err != nil {
		return nil, nil, err
	}

	return m.strategy.Prepare(ctx, m.runtime, script)
}

func (m *module) Close() error {
	return m.strategy.Close()
}
