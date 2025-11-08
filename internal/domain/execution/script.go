package execution

// Language identifies the implementation language of a Script.
type Language string

const (
	// LanguagePython represents scripts written in Python.
	LanguagePython Language = "python"
	// LanguageGo represents scripts written in Go.
	LanguageGo Language = "go"
	// LanguageC represents scripts written in C.
	LanguageC Language = "c"
)

// Script represents a unit of source code ready for execution.
type Script struct {
	ID       string
	Language Language
	Source   string
	Limits   RunLimits
	Tests    []TestCase
}

// RunReport captures the outcome of executing a Script.
type RunReport struct {
	Script Script
	Result *Result
	Err    error
}
