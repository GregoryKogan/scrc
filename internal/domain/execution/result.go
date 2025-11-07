package execution

import "time"

// Status identifies the high-level outcome of a script execution.
type Status string

const (
	// StatusOK indicates that the script finished within the configured limits.
	StatusOK Status = "OK"
	// StatusTimeLimit indicates that the script was terminated for exceeding the time limit.
	StatusTimeLimit Status = "TL"
	// StatusMemoryLimit indicates that the script was terminated for exceeding the memory limit.
	StatusMemoryLimit Status = "ML"
)

// Result captures the outcome of executing a script.
type Result struct {
	Status   Status
	Stdout   string
	Stderr   string
	ExitCode int64
	Duration time.Duration
}
