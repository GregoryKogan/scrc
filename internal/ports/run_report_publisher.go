package ports

import (
	"context"

	"scrc/internal/domain/execution"
)

// RunReportPublisher publishes script execution reports to an external system.
type RunReportPublisher interface {
	PublishRunReport(ctx context.Context, report execution.RunReport) error
	Close() error
}
