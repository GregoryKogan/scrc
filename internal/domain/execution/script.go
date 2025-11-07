package execution

// Script represents a unit of Python source code ready for execution.
type Script struct {
	ID     string
	Source string
	Limits RunLimits
}

// RunReport captures the outcome of executing a Script.
type RunReport struct {
	Script Script
	Result *Result
	Err    error
}
