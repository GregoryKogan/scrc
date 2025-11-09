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
	cppSourceFilename    = "main.cpp"
	cppBinaryFilename    = "program"
	javaSourceFilename   = "Main.java"
	javaBinaryFilename   = "program.jar"
)

func strategyForLanguage(lang execution.Language) (languageStrategy, error) {
	switch lang {
	case execution.LanguagePython:
		return &pythonStrategy{}, nil
	case execution.LanguageGo:
		return &goStrategy{}, nil
	case execution.LanguageC:
		return &cStrategy{}, nil
	case execution.LanguageCPP:
		return &cppStrategy{}, nil
	case execution.LanguageJava:
		return &javaStrategy{}, nil
	default:
		return nil, fmt.Errorf("docker runtime: no strategy registered for language %q", lang)
	}
}
