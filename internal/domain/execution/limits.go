package execution

import "time"

// RunLimits describes optional resource boundaries for a single script execution.
//
// A zero value RunLimits imposes no additional restrictions.
type RunLimits struct {
	// TimeLimit caps how long the script is allowed to run. Zero means no limit.
	TimeLimit time.Duration
	// MemoryLimitBytes caps the container memory usage in bytes. Zero means no limit.
	MemoryLimitBytes int64
}
