package otlpeek

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// Global singleton for the webserver
var (
	globalServer     *http.Server
	globalServerOnce sync.Once
	globalData       *sharedData
)

// sharedData holds the data that all exporter instances share
type sharedData struct {
	mu           sync.RWMutex
	traces       map[string][]string // key: resource attributes string, value: gRPC messages
	metrics      map[string][]string // key: resource attributes string, value: gRPC messages
	logs         map[string][]string // key: resource attributes string, value: gRPC messages
	lastActivity time.Time
}

// Exporter implements the OTLP stream viewer
type Exporter struct {
	config *Config
	logger *zap.Logger
}

// newExporter creates a new otlpeek exporter
func newExporter(cfg *Config, set component.TelemetrySettings) (*Exporter, error) {
	exp := &Exporter{
		config: cfg,
		logger: set.Logger,
	}

	return exp, nil
}

// start starts the web server (singleton)
func (e *Exporter) start(ctx context.Context, host component.Host) error {
	globalServerOnce.Do(func() {
		// Initialize global data
		globalData = &sharedData{
			traces:       make(map[string][]string),
			metrics:      make(map[string][]string),
			logs:         make(map[string][]string),
			lastActivity: time.Now(),
		}

		// Create and start the webserver
		mux := http.NewServeMux()
		mux.HandleFunc("/", e.handleRoot)
		mux.HandleFunc("/traces", e.handleTraces)
		mux.HandleFunc("/metrics", e.handleMetrics)
		mux.HandleFunc("/logs", e.handleLogs)

		globalServer = &http.Server{
			Addr:    e.config.Endpoint,
			Handler: mux,
		}

		go func() {
			e.logger.Info("Starting otlpeek web server", zap.String("endpoint", e.config.Endpoint))
			if err := globalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				e.logger.Error("Failed to start server", zap.Error(err))
			}
		}()
	})

	return nil
}

// shutdown stops the web server
func (e *Exporter) shutdown(ctx context.Context) error {
	if globalServer != nil {
		return globalServer.Shutdown(ctx)
	}
	return nil
}

// pushTraces processes trace data
func (e *Exporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	globalData.mu.Lock()
	defer globalData.mu.Unlock()

	globalData.lastActivity = time.Now()

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		resourceAttrs := rs.Resource().Attributes().AsRaw()
		resourceKey := fmt.Sprintf("%v", resourceAttrs)

		// Count total spans across all scope spans for this resource
		totalSpans := 0
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			totalSpans += ss.Spans().Len()
		}

		// Create one summary entry for this gRPC message
		entry := fmt.Sprintf("[%s] Traces: %d spans",
			time.Now().Format("15:04:05"), totalSpans)

		// Initialize the resource group if it doesn't exist
		if globalData.traces[resourceKey] == nil {
			globalData.traces[resourceKey] = []string{}
		}
		globalData.traces[resourceKey] = append(globalData.traces[resourceKey], entry)

		// Add detailed information as sub-entries
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			scopeName := ss.Scope().Name()
			scopeVersion := ss.Scope().Version()

			scopeEntry := fmt.Sprintf("  Scope: %s (version: %s)", scopeName, scopeVersion)
			globalData.traces[resourceKey] = append(globalData.traces[resourceKey], scopeEntry)

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				spanAttrs := span.Attributes().AsRaw()

				spanEntry := fmt.Sprintf("    Span: %s (ID: %x, TraceID: %x)",
					span.Name(), span.SpanID(), span.TraceID())
				globalData.traces[resourceKey] = append(globalData.traces[resourceKey], spanEntry)

				if len(spanAttrs) > 0 {
					attrEntry := fmt.Sprintf("      Attributes: %v", spanAttrs)
					globalData.traces[resourceKey] = append(globalData.traces[resourceKey], attrEntry)
				}

				if span.Kind() != ptrace.SpanKindUnspecified {
					kindEntry := fmt.Sprintf("      Kind: %s", span.Kind().String())
					globalData.traces[resourceKey] = append(globalData.traces[resourceKey], kindEntry)
				}

				if span.Status().Code() != ptrace.StatusCodeUnset {
					statusEntry := fmt.Sprintf("      Status: %s - %s",
						span.Status().Code().String(), span.Status().Message())
					globalData.traces[resourceKey] = append(globalData.traces[resourceKey], statusEntry)
				}
			}
		}

		// Keep only last 100 entries per resource group
		if len(globalData.traces[resourceKey]) > 100 {
			globalData.traces[resourceKey] = globalData.traces[resourceKey][len(globalData.traces[resourceKey])-100:]
		}
	}

	e.logger.Debug("Received traces", zap.Int("count", td.SpanCount()))
	return nil
}

// pushMetrics processes metric data
func (e *Exporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {
	globalData.mu.Lock()
	defer globalData.mu.Unlock()

	globalData.lastActivity = time.Now()

	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		resourceAttrs := rm.Resource().Attributes().AsRaw()
		resourceKey := fmt.Sprintf("%v", resourceAttrs)

		// Count total metrics across all scope metrics for this resource
		totalMetrics := 0
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			totalMetrics += sm.Metrics().Len()
		}

		// Create one summary entry for this gRPC message
		entry := fmt.Sprintf("[%s] Metrics: %d metrics",
			time.Now().Format("15:04:05"), totalMetrics)

		// Initialize the resource group if it doesn't exist
		if globalData.metrics[resourceKey] == nil {
			globalData.metrics[resourceKey] = []string{}
		}
		globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], entry)

		// Add detailed information as sub-entries
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			scopeName := sm.Scope().Name()
			scopeVersion := sm.Scope().Version()

			scopeEntry := fmt.Sprintf("  Scope: %s (version: %s)", scopeName, scopeVersion)
			globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], scopeEntry)

			for k := 0; k < sm.Metrics().Len(); k++ {
				metric := sm.Metrics().At(k)
				metricEntry := fmt.Sprintf("    Metric: %s (%s)", metric.Name(), metric.Type().String())
				globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], metricEntry)

				// Add metric description if available
				if metric.Description() != "" {
					descEntry := fmt.Sprintf("      Description: %s", metric.Description())
					globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], descEntry)
				}

				// Add unit if available
				if metric.Unit() != "" {
					unitEntry := fmt.Sprintf("      Unit: %s", metric.Unit())
					globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], unitEntry)
				}

				// Process different metric types
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dps := metric.Gauge().DataPoints()
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpAttrs := dp.Attributes().AsRaw()
						dpEntry := fmt.Sprintf("      DataPoint: %v (attrs: %v)", dp.DoubleValue(), dpAttrs)
						globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], dpEntry)
					}
				case pmetric.MetricTypeSum:
					dps := metric.Sum().DataPoints()
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpAttrs := dp.Attributes().AsRaw()
						dpEntry := fmt.Sprintf("      DataPoint: %v (attrs: %v)", dp.DoubleValue(), dpAttrs)
						globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], dpEntry)
					}
				case pmetric.MetricTypeHistogram:
					dps := metric.Histogram().DataPoints()
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpAttrs := dp.Attributes().AsRaw()
						dpEntry := fmt.Sprintf("      DataPoint: count=%d, sum=%v (attrs: %v)",
							dp.Count(), dp.Sum(), dpAttrs)
						globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], dpEntry)
					}
				case pmetric.MetricTypeExponentialHistogram:
					dps := metric.ExponentialHistogram().DataPoints()
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpAttrs := dp.Attributes().AsRaw()
						dpEntry := fmt.Sprintf("      DataPoint: count=%d, sum=%v (attrs: %v)",
							dp.Count(), dp.Sum(), dpAttrs)
						globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], dpEntry)
					}
				case pmetric.MetricTypeSummary:
					dps := metric.Summary().DataPoints()
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpAttrs := dp.Attributes().AsRaw()
						dpEntry := fmt.Sprintf("      DataPoint: count=%d, sum=%v (attrs: %v)",
							dp.Count(), dp.Sum(), dpAttrs)
						globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], dpEntry)
					}
				}
			}
		}

		// Keep only last 100 entries per resource group
		if len(globalData.metrics[resourceKey]) > 100 {
			globalData.metrics[resourceKey] = globalData.metrics[resourceKey][len(globalData.metrics[resourceKey])-100:]
		}
	}

	e.logger.Debug("Received metrics", zap.Int("count", md.MetricCount()))
	return nil
}

// pushLogs processes log data
func (e *Exporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	globalData.mu.Lock()
	defer globalData.mu.Unlock()

	globalData.lastActivity = time.Now()

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		resourceAttrs := rl.Resource().Attributes().AsRaw()
		resourceKey := fmt.Sprintf("%v", resourceAttrs)

		// Count total log records across all scope logs for this resource
		totalLogs := 0
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			totalLogs += sl.LogRecords().Len()
		}

		// Create one summary entry for this gRPC message
		entry := fmt.Sprintf("[%s] Logs: %d log records",
			time.Now().Format("15:04:05"), totalLogs)

		// Initialize the resource group if it doesn't exist
		if globalData.logs[resourceKey] == nil {
			globalData.logs[resourceKey] = []string{}
		}
		globalData.logs[resourceKey] = append(globalData.logs[resourceKey], entry)

		// Add detailed information as sub-entries
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			scopeName := sl.Scope().Name()
			scopeVersion := sl.Scope().Version()

			scopeEntry := fmt.Sprintf("  Scope: %s (version: %s)", scopeName, scopeVersion)
			globalData.logs[resourceKey] = append(globalData.logs[resourceKey], scopeEntry)

			for k := 0; k < sl.LogRecords().Len(); k++ {
				logRecord := sl.LogRecords().At(k)

				// Basic log record info
				logEntry := fmt.Sprintf("    Log: %s", logRecord.Body().AsString())
				globalData.logs[resourceKey] = append(globalData.logs[resourceKey], logEntry)

				// Timestamp
				if logRecord.Timestamp() != 0 {
					tsEntry := fmt.Sprintf("      Timestamp: %s",
						time.Unix(0, int64(logRecord.Timestamp())).Format("2006-01-02 15:04:05.000000000"))
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], tsEntry)
				}

				// Observed timestamp
				if logRecord.ObservedTimestamp() != 0 {
					otsEntry := fmt.Sprintf("      Observed: %s",
						time.Unix(0, int64(logRecord.ObservedTimestamp())).Format("2006-01-02 15:04:05.000000000"))
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], otsEntry)
				}

				// Severity
				if logRecord.SeverityNumber() != plog.SeverityNumberUnspecified {
					sevEntry := fmt.Sprintf("      Severity: %s (%s)",
						logRecord.SeverityNumber().String(), logRecord.SeverityText())
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], sevEntry)
				}

				// Attributes
				logAttrs := logRecord.Attributes().AsRaw()
				if len(logAttrs) > 0 {
					attrEntry := fmt.Sprintf("      Attributes: %v", logAttrs)
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], attrEntry)
				}

				// Trace and span context
				if len(logRecord.TraceID()) > 0 {
					traceEntry := fmt.Sprintf("      TraceID: %x", logRecord.TraceID())
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], traceEntry)
				}
				if len(logRecord.SpanID()) > 0 {
					spanEntry := fmt.Sprintf("      SpanID: %x", logRecord.SpanID())
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], spanEntry)
				}

				// Event name if present
				if logRecord.EventName() != "" {
					eventEntry := fmt.Sprintf("      Event: %s", logRecord.EventName())
					globalData.logs[resourceKey] = append(globalData.logs[resourceKey], eventEntry)
				}
			}
		}

		// Keep only last 100 entries per resource group
		if len(globalData.logs[resourceKey]) > 100 {
			globalData.logs[resourceKey] = globalData.logs[resourceKey][len(globalData.logs[resourceKey])-100:]
		}
	}

	e.logger.Debug("Received logs", zap.Int("count", ld.LogRecordCount()))
	return nil
}

// countMainEntries counts only the main gRPC message entries (lines starting with [time])
func countMainEntries(entries []string) int {
	count := 0
	for _, entry := range entries {
		if len(entry) > 0 && entry[0] == '[' {
			count++
		}
	}
	return count
}

// countTotalMainEntries counts main entries across all resource groups
func countTotalMainEntries(resourceGroups map[string][]string) int {
	total := 0
	for _, entries := range resourceGroups {
		total += countMainEntries(entries)
	}
	return total
}

// HTTP handlers
func (e *Exporter) handleRoot(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "OTLPeek - OpenTelemetry Stream Viewer\n")
	fmt.Fprintf(w, "Last activity: %s\n", globalData.lastActivity.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "\nSummary:\n")
	fmt.Fprintf(w, "  Traces:  %d entries  (browse: /traces)\n", countTotalMainEntries(globalData.traces))
	fmt.Fprintf(w, "  Metrics: %d entries  (browse: /metrics)\n", countTotalMainEntries(globalData.metrics))
	fmt.Fprintf(w, "  Logs:    %d entries  (browse: /logs)\n", countTotalMainEntries(globalData.logs))
	fmt.Fprintf(w, "\nUse /traces, /metrics, or /logs to view details.\n")
}

func (e *Exporter) handleTraces(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== TRACES ===\n\n")

	if len(globalData.traces) == 0 {
		fmt.Fprintf(w, "No traces received yet.\n")
		return
	}

	// Display traces grouped by resource
	for resourceKey, entries := range globalData.traces {
		fmt.Fprintf(w, "Resource: %s\n", resourceKey)
		for i := len(entries) - 1; i >= 0; i-- {
			fmt.Fprintf(w, "%s\n", entries[i])
		}
		fmt.Fprintf(w, "\n")
	}
}

func (e *Exporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== METRICS ===\n\n")

	if len(globalData.metrics) == 0 {
		fmt.Fprintf(w, "No metrics received yet.\n")
		return
	}

	// Display metrics grouped by resource
	for resourceKey, entries := range globalData.metrics {
		fmt.Fprintf(w, "Resource: %s\n", resourceKey)
		for i := len(entries) - 1; i >= 0; i-- {
			fmt.Fprintf(w, "%s\n", entries[i])
		}
		fmt.Fprintf(w, "\n")
	}
}

func (e *Exporter) handleLogs(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== LOGS ===\n\n")

	if len(globalData.logs) == 0 {
		fmt.Fprintf(w, "No logs received yet.\n")
		return
	}

	// Display logs grouped by resource
	for resourceKey, entries := range globalData.logs {
		fmt.Fprintf(w, "Resource: %s\n", resourceKey)
		for i := len(entries) - 1; i >= 0; i-- {
			fmt.Fprintf(w, "%s\n", entries[i])
		}
		fmt.Fprintf(w, "\n")
	}
}
