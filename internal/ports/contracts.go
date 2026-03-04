package ports

import (
	"context"

	"github.com/juanmferreira93/iracing-agent/internal/domain"
)

type TelemetryParser interface {
	ParseFile(ctx context.Context, filePath string) (*domain.UploadBundle, error)
}

type TelemetryUploader interface {
	UploadTelemetry(ctx context.Context, payload domain.UploadBundle) error
}
