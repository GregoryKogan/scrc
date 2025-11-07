package execution

import "time"

// TestCase describes a single stdin/stdout expectation pair for a script.
type TestCase struct {
	Number         int
	Input          string
	ExpectedOutput string
}

// TestResult captures the outcome of executing a single TestCase.
type TestResult struct {
	Case     TestCase
	Status   Status
	Stdout   string
	Stderr   string
	ExitCode int64
	Duration time.Duration
	Error    string
}
