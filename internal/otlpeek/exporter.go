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
	traces       []string
	metrics      []string
	logs         []string
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
			traces:       make([]string, 0),
			metrics:      make([]string, 0),
			logs:         make([]string, 0),
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

		entry := fmt.Sprintf("[%s] Resource: %v, Spans: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rs.ScopeSpans().Len())
		globalData.traces = append(globalData.traces, entry)
	}

	// Keep only last 100 entries
	if len(globalData.traces) > 100 {
		globalData.traces = globalData.traces[len(globalData.traces)-100:]
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

		entry := fmt.Sprintf("[%s] Resource: %v, Metrics: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rm.ScopeMetrics().Len())
		globalData.metrics = append(globalData.metrics, entry)
	}

	// Keep only last 100 entries
	if len(globalData.metrics) > 100 {
		globalData.metrics = globalData.metrics[len(globalData.metrics)-100:]
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

		entry := fmt.Sprintf("[%s] Resource: %v, Logs: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rl.ScopeLogs().Len())
		globalData.logs = append(globalData.logs, entry)
	}

	// Keep only last 100 entries
	if len(globalData.logs) > 100 {
		globalData.logs = globalData.logs[len(globalData.logs)-100:]
	}

	e.logger.Debug("Received logs", zap.Int("count", ld.LogRecordCount()))
	return nil
}

// HTTP handlers
func (e *Exporter) handleRoot(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "OTLPeek - OpenTelemetry Stream Viewer\n")
	fmt.Fprintf(w, "Last activity: %s\n", globalData.lastActivity.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "\nSummary:\n")
	fmt.Fprintf(w, "  Traces:  %d entries  (browse: /traces)\n", len(globalData.traces))
	fmt.Fprintf(w, "  Metrics: %d entries  (browse: /metrics)\n", len(globalData.metrics))
	fmt.Fprintf(w, "  Logs:    %d entries  (browse: /logs)\n", len(globalData.logs))
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

	for i := len(globalData.traces) - 1; i >= 0; i-- {
		fmt.Fprintf(w, "%s\n", globalData.traces[i])
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

	for i := len(globalData.metrics) - 1; i >= 0; i-- {
		fmt.Fprintf(w, "%s\n", globalData.metrics[i])
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

	for i := len(globalData.logs) - 1; i >= 0; i-- {
		fmt.Fprintf(w, "%s\n", globalData.logs[i])
	}
}
