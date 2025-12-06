package docker

import "scrc/internal/domain/execution"

// Config describes how to create a Docker-backed runtime engine.
type Config struct {
	Languages     map[execution.Language]LanguageConfig
	DefaultLimits execution.RunLimits
}

// LanguageConfig specifies container settings for a single language.
type LanguageConfig struct {
	Image    string
	RunImage string // Optional: image to use for running (defaults to Image if empty). Useful for compiled languages where build and run images differ.
	Workdir  string
}
