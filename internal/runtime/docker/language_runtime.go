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
	// Default RunImage to Image if not specified
	if cfg.RunImage == "" {
		cfg.RunImage = cfg.Image
	}
	return &languageRuntime{
		language: lang,
		config:   cfg,
		engine:   engine,
	}, nil
}

func (l *languageRuntime) ensureImage(ctx context.Context) error {
	l.pullOnce.Do(func() {
		// Pull both build and run images
		if err := l.engine.pullImage(ctx, l.config.Image); err != nil {
			l.pullErr = err
			return
		}
		if l.config.RunImage != l.config.Image {
			if err := l.engine.pullImage(ctx, l.config.RunImage); err != nil {
				l.pullErr = err
				return
			}
		}
	})
	return l.pullErr
}

// runImage returns the image to use for running programs
func (l *languageRuntime) runImage() string {
	if l.config.RunImage != "" {
		return l.config.RunImage
	}
	return l.config.Image
}
