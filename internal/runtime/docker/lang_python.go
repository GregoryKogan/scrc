package docker

import (
	"context"

	"scrc/internal/domain/execution"
	runtimex "scrc/internal/runtime"
)

type pythonStrategy struct{}

func (p *pythonStrategy) Prepare(ctx context.Context, lang *languageRuntime, script execution.Script) (runtimex.PreparedScript, *execution.Result, error) {
	return &pythonPreparedScript{
		runtime: lang,
		script:  script,
	}, nil, nil
}

func (p *pythonStrategy) Close() error {
	return nil
}

type pythonPreparedScript struct {
	runtime *languageRuntime
	script  execution.Script
}

func (p *pythonPreparedScript) Run(ctx context.Context, stdin string) (*execution.Result, error) {
	return p.runtime.engine.runProgram(ctx, p.runtime, p.script.Limits, []string{"python", pythonScriptFilename}, []fileSpec{
		{
			Name: pythonScriptFilename,
			Mode: 0o644,
			Data: []byte(p.script.Source),
		},
	}, stdin, true)
}

func (p *pythonPreparedScript) Close() error {
	return nil
}
