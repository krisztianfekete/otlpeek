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

// Exporter implements the OTLP stream viewer
type Exporter struct {
	config *Config
	logger *zap.Logger

	mu           sync.RWMutex
	traces       []string
	metrics      []string
	logs         []string
	server       *http.Server
	lastActivity time.Time
}

// newExporter creates a new otlpeek exporter
func newExporter(cfg *Config, set component.TelemetrySettings) (*Exporter, error) {
	exp := &Exporter{
		config:       cfg,
		logger:       set.Logger,
		traces:       make([]string, 0),
		metrics:      make([]string, 0),
		logs:         make([]string, 0),
		lastActivity: time.Now(),
	}

	return exp, nil
}

// start starts the web server
func (e *Exporter) start(ctx context.Context, host component.Host) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", e.handleRoot)
	mux.HandleFunc("/traces", e.handleTraces)
	mux.HandleFunc("/metrics", e.handleMetrics)
	mux.HandleFunc("/logs", e.handleLogs)

	e.server = &http.Server{
		Addr:    e.config.Endpoint,
		Handler: mux,
	}

	go func() {
		e.logger.Info("Starting otlpeek web server", zap.String("endpoint", e.config.Endpoint))
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.logger.Error("Failed to start server", zap.Error(err))
		}
	}()

	return nil
}

// shutdown stops the web server
func (e *Exporter) shutdown(ctx context.Context) error {
	if e.server != nil {
		return e.server.Shutdown(ctx)
	}
	return nil
}

// pushTraces processes trace data
func (e *Exporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastActivity = time.Now()

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		resourceAttrs := rs.Resource().Attributes().AsRaw()

		entry := fmt.Sprintf("[%s] Resource: %v, Spans: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rs.ScopeSpans().Len())
		e.traces = append(e.traces, entry)
	}

	// Keep only last 100 entries
	if len(e.traces) > 100 {
		e.traces = e.traces[len(e.traces)-100:]
	}

	e.logger.Debug("Received traces", zap.Int("count", td.SpanCount()))
	return nil
}

// pushMetrics processes metric data
func (e *Exporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastActivity = time.Now()

	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		resourceAttrs := rm.Resource().Attributes().AsRaw()

		entry := fmt.Sprintf("[%s] Resource: %v, Metrics: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rm.ScopeMetrics().Len())
		e.metrics = append(e.metrics, entry)
	}

	// Keep only last 100 entries
	if len(e.metrics) > 100 {
		e.metrics = e.metrics[len(e.metrics)-100:]
	}

	e.logger.Debug("Received metrics", zap.Int("count", md.MetricCount()))
	return nil
}

// pushLogs processes log data
func (e *Exporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastActivity = time.Now()

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		resourceAttrs := rl.Resource().Attributes().AsRaw()

		entry := fmt.Sprintf("[%s] Resource: %v, Logs: %d",
			time.Now().Format("15:04:05"), resourceAttrs, rl.ScopeLogs().Len())
		fmt.Printf("Appending log entry: %q\n", entry)
		e.logs = append(e.logs, entry)
	}

	// Keep only last 100 entries
	if len(e.logs) > 100 {
		e.logs = e.logs[len(e.logs)-100:]
	}

	e.logger.Debug("Received logs", zap.Int("count", ld.LogRecordCount()))
	return nil
}

// HTTP handlers
func (e *Exporter) handleRoot(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "OTLPeek - OpenTelemetry Stream Viewer\n")
	fmt.Fprintf(w, "Last activity: %s\n", e.lastActivity.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "\nSummary:\n")
	fmt.Fprintf(w, "  Traces:  %d entries  (browse: /traces)\n", len(e.traces))
	fmt.Fprintf(w, "  Metrics: %d entries  (browse: /metrics)\n", len(e.metrics))
	fmt.Fprintf(w, "  Logs:    %d entries  (browse: /logs)\n", len(e.logs))
	fmt.Fprintf(w, "\nUse /traces, /metrics, or /logs to view details.\n")
}

func (e *Exporter) handleTraces(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== TRACES ===\n\n")

	if len(e.traces) == 0 {
		fmt.Fprintf(w, "No traces received yet.\n")
		return
	}

	for i := len(e.traces) - 1; i >= 0; i-- {
		fmt.Fprintf(w, "%s\n", e.traces[i])
	}
}

func (e *Exporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== METRICS ===\n\n")

	if len(e.metrics) == 0 {
		fmt.Fprintf(w, "No metrics received yet.\n")
		return
	}

	for i := len(e.metrics) - 1; i >= 0; i-- {
		fmt.Fprintf(w, "%s\n", e.metrics[i])
	}
}

func (e *Exporter) handleLogs(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "=== LOGS ===\n\n")

	fmt.Printf("handleLogs: e.logs has %d entries\n", len(e.logs))

	if len(e.logs) == 0 {
		fmt.Fprintf(w, "No logs received yet.\n")
		return
	}

	for i := len(e.logs) - 1; i >= 0; i-- {
		fmt.Printf("handleLogs: displaying entry: %q\n", e.logs[i])
		fmt.Fprintf(w, "%s\n", e.logs[i])
	}
}
