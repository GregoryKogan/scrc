package docker

import (
	"fmt"

	"scrc/internal/domain/execution"
)

const (
	pythonScriptFilename = "script.py"
	goSourceFilename     = "main.go"
	goBinaryFilename     = "program"
	cSourceFilename      = "main.c"
	cBinaryFilename      = "program"
)

func strategyForLanguage(lang execution.Language) (languageStrategy, error) {
	switch lang {
	case execution.LanguagePython:
		return &pythonStrategy{}, nil
	case execution.LanguageGo:
		return &goStrategy{}, nil
	case execution.LanguageC:
		return &cStrategy{}, nil
	default:
		return nil, fmt.Errorf("docker runtime: no strategy registered for language %q", lang)
	}
}
