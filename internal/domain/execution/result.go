package execution

import "time"

// Result captures the outcome of executing a script.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int64
	Duration time.Duration
}
