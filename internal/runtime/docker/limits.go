package docker

import "scrc/internal/domain/execution"

func normalizeLimits(l execution.RunLimits) execution.RunLimits {
	if l.TimeLimit < 0 {
		l.TimeLimit = 0
	}
	if l.MemoryLimitBytes < 0 {
		l.MemoryLimitBytes = 0
	}
	return l
}
