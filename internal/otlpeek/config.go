package otlpeek

import (
	"go.opentelemetry.io/collector/component"
)

// Config represents the configuration for the otlpeek exporter
type Config struct {
	// Endpoint is the address where the web server will listen
	Endpoint string `mapstructure:"endpoint"`
}

// Validate checks if the configuration is valid
func (cfg *Config) Validate() error {
	return nil
}

var _ component.Config = (*Config)(nil)
