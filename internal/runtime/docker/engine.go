package docker

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/docker/client"

	"scrc/internal/domain/execution"
	runtimex "scrc/internal/runtime"
)

// Engine implements runtime.Engine backed by Docker containers.
type Engine struct {
	registry *runtimex.Registry
	client   dockerClient
}

// New constructs an Engine using the supplied configuration.
func New(cfg Config) (runtimex.Engine, error) {
	if len(cfg.Languages) == 0 {
		return nil, fmt.Errorf("docker runtime: at least one language must be configured")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker runtime: create client: %w", err)
	}

	engine, err := newEngineWithClient(cli, cfg)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}

	return engine, nil
}

// Prepare delegates to the underlying registry.
func (e *Engine) Prepare(ctx context.Context, script execution.Script) (runtimex.PreparedScript, *execution.Result, error) {
	return e.registry.Prepare(ctx, script)
}

// Close releases module resources and the Docker client.
func (e *Engine) Close() error {
	var errs []error
	if err := e.registry.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := e.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("docker client: %w", err))
	}
	return errors.Join(errs...)
}

func newEngineWithClient(cli dockerClient, cfg Config) (*Engine, error) {
	env := newContainerEngine(cli, cfg.DefaultLimits)

	modules := make([]runtimex.Module, 0, len(cfg.Languages))
	for lang, langCfg := range cfg.Languages {
		module, err := newModule(lang, langCfg, env)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}

	registry, err := runtimex.NewRegistry(modules...)
	if err != nil {
		return nil, err
	}

	return &Engine{
		registry: registry,
		client:   cli,
	}, nil
}
