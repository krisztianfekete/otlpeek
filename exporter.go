package otlpeek

import (
	"github.com/krisztianfekete/otlpeek/internal/otlpeek"
	"go.opentelemetry.io/collector/exporter"
)

// NewFactory returns a new factory for the otlpeek exporter.
func NewFactory() exporter.Factory {
	return otlpeek.NewFactory()
}
