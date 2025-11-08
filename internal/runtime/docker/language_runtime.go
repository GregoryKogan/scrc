package docker

import (
	"context"
	"fmt"
	"sync"

	"scrc/internal/domain/execution"
)

type languageRuntime struct {
	language execution.Language
	config   LanguageConfig
	engine   *containerEngine

	pullOnce sync.Once
	pullErr  error
}

func newLanguageRuntime(lang execution.Language, cfg LanguageConfig, engine *containerEngine) (*languageRuntime, error) {
	if cfg.Image == "" {
		return nil, fmt.Errorf("docker runtime: language %q missing image configuration", lang)
	}
	if cfg.Workdir == "" {
		cfg.Workdir = "/tmp"
	}
	return &languageRuntime{
		language: lang,
		config:   cfg,
		engine:   engine,
	}, nil
}

func (l *languageRuntime) ensureImage(ctx context.Context) error {
	l.pullOnce.Do(func() {
		l.pullErr = l.engine.pullImage(ctx, l.config.Image)
	})
	return l.pullErr
}
