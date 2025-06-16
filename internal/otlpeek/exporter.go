package otlpeek

import (
	"context"
	"encoding/json"
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
	traces       map[string][]map[string]interface{} // key: resource attributes string, value: raw gRPC messages
	metrics      map[string][]map[string]interface{} // key: resource attributes string, value: raw gRPC messages
	logs         map[string][]map[string]interface{} // key: resource attributes string, value: raw gRPC messages
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
			traces:       make(map[string][]map[string]interface{}),
			metrics:      make(map[string][]map[string]interface{}),
			logs:         make(map[string][]map[string]interface{}),
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
			e.logger.Info("Starting otlpeekexporter web server", zap.String("endpoint", e.config.Endpoint))
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

		// Convert the resource spans to a map for JSON serialization
		resourceSpansMap := map[string]interface{}{
			"resource": map[string]interface{}{
				"attributes": resourceAttrs,
			},
			"scopeSpans": []map[string]interface{}{},
			"timestamp":  time.Now().Format(time.RFC3339Nano),
		}

		// Add scope spans
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			scopeSpansMap := map[string]interface{}{
				"scope": map[string]interface{}{
					"name":    ss.Scope().Name(),
					"version": ss.Scope().Version(),
				},
				"spans": []map[string]interface{}{},
			}

			// Add spans
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				spanMap := map[string]interface{}{
					"traceId":           fmt.Sprintf("%x", span.TraceID()),
					"spanId":            fmt.Sprintf("%x", span.SpanID()),
					"parentSpanId":      fmt.Sprintf("%x", span.ParentSpanID()),
					"name":              span.Name(),
					"kind":              span.Kind().String(),
					"startTimeUnixNano": span.StartTimestamp(),
					"endTimeUnixNano":   span.EndTimestamp(),
					"attributes":        span.Attributes().AsRaw(),
					"status": map[string]interface{}{
						"code":    span.Status().Code().String(),
						"message": span.Status().Message(),
					},
				}
				scopeSpansMap["spans"] = append(scopeSpansMap["spans"].([]map[string]interface{}), spanMap)
			}

			resourceSpansMap["scopeSpans"] = append(resourceSpansMap["scopeSpans"].([]map[string]interface{}), scopeSpansMap)
		}

		// Initialize the resource group if it doesn't exist
		if globalData.traces[resourceKey] == nil {
			globalData.traces[resourceKey] = []map[string]interface{}{}
		}
		globalData.traces[resourceKey] = append(globalData.traces[resourceKey], resourceSpansMap)

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

		// Convert the resource metrics to a map for JSON serialization
		resourceMetricsMap := map[string]interface{}{
			"resource": map[string]interface{}{
				"attributes": resourceAttrs,
			},
			"scopeMetrics": []map[string]interface{}{},
			"timestamp":    time.Now().Format(time.RFC3339Nano),
		}

		// Add scope metrics
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			scopeMetricsMap := map[string]interface{}{
				"scope": map[string]interface{}{
					"name":    sm.Scope().Name(),
					"version": sm.Scope().Version(),
				},
				"metrics": []map[string]interface{}{},
			}

			// Add metrics
			for k := 0; k < sm.Metrics().Len(); k++ {
				metric := sm.Metrics().At(k)
				metricMap := map[string]interface{}{
					"name":        metric.Name(),
					"description": metric.Description(),
					"unit":        metric.Unit(),
					"type":        metric.Type().String(),
				}

				// Add metric data based on type
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dps := metric.Gauge().DataPoints()
					dataPoints := []map[string]interface{}{}
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpMap := map[string]interface{}{
							"timeUnixNano": dp.Timestamp(),
							"value":        dp.DoubleValue(),
							"attributes":   dp.Attributes().AsRaw(),
						}
						dataPoints = append(dataPoints, dpMap)
					}
					metricMap["dataPoints"] = dataPoints
				case pmetric.MetricTypeSum:
					dps := metric.Sum().DataPoints()
					dataPoints := []map[string]interface{}{}
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpMap := map[string]interface{}{
							"timeUnixNano": dp.Timestamp(),
							"value":        dp.DoubleValue(),
							"attributes":   dp.Attributes().AsRaw(),
						}
						dataPoints = append(dataPoints, dpMap)
					}
					metricMap["dataPoints"] = dataPoints
				case pmetric.MetricTypeHistogram:
					dps := metric.Histogram().DataPoints()
					dataPoints := []map[string]interface{}{}
					for l := 0; l < dps.Len(); l++ {
						dp := dps.At(l)
						dpMap := map[string]interface{}{
							"timeUnixNano": dp.Timestamp(),
							"count":        dp.Count(),
							"sum":          dp.Sum(),
							"attributes":   dp.Attributes().AsRaw(),
						}
						dataPoints = append(dataPoints, dpMap)
					}
					metricMap["dataPoints"] = dataPoints
				}

				scopeMetricsMap["metrics"] = append(scopeMetricsMap["metrics"].([]map[string]interface{}), metricMap)
			}

			resourceMetricsMap["scopeMetrics"] = append(resourceMetricsMap["scopeMetrics"].([]map[string]interface{}), scopeMetricsMap)
		}

		// Initialize the resource group if it doesn't exist
		if globalData.metrics[resourceKey] == nil {
			globalData.metrics[resourceKey] = []map[string]interface{}{}
		}
		globalData.metrics[resourceKey] = append(globalData.metrics[resourceKey], resourceMetricsMap)

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

		// Convert the resource logs to a map for JSON serialization
		resourceLogsMap := map[string]interface{}{
			"resource": map[string]interface{}{
				"attributes": resourceAttrs,
			},
			"scopeLogs": []map[string]interface{}{},
			"timestamp": time.Now().Format(time.RFC3339Nano),
		}

		// Add scope logs
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			scopeLogsMap := map[string]interface{}{
				"scope": map[string]interface{}{
					"name":    sl.Scope().Name(),
					"version": sl.Scope().Version(),
				},
				"logRecords": []map[string]interface{}{},
			}

			// Add log records
			for k := 0; k < sl.LogRecords().Len(); k++ {
				logRecord := sl.LogRecords().At(k)
				logRecordMap := map[string]interface{}{
					"timeUnixNano":         logRecord.Timestamp(),
					"observedTimeUnixNano": logRecord.ObservedTimestamp(),
					"severityNumber":       logRecord.SeverityNumber().String(),
					"severityText":         logRecord.SeverityText(),
					"body":                 logRecord.Body().AsString(),
					"attributes":           logRecord.Attributes().AsRaw(),
					"traceId":              fmt.Sprintf("%x", logRecord.TraceID()),
					"spanId":               fmt.Sprintf("%x", logRecord.SpanID()),
					"eventName":            logRecord.EventName(),
				}
				scopeLogsMap["logRecords"] = append(scopeLogsMap["logRecords"].([]map[string]interface{}), logRecordMap)
			}

			resourceLogsMap["scopeLogs"] = append(resourceLogsMap["scopeLogs"].([]map[string]interface{}), scopeLogsMap)
		}

		// Initialize the resource group if it doesn't exist
		if globalData.logs[resourceKey] == nil {
			globalData.logs[resourceKey] = []map[string]interface{}{}
		}
		globalData.logs[resourceKey] = append(globalData.logs[resourceKey], resourceLogsMap)

		// Keep only last 100 entries per resource group
		if len(globalData.logs[resourceKey]) > 100 {
			globalData.logs[resourceKey] = globalData.logs[resourceKey][len(globalData.logs[resourceKey])-100:]
		}
	}

	e.logger.Debug("Received logs", zap.Int("count", ld.LogRecordCount()))
	return nil
}

// countTotalEntries counts total entries across all resource groups
func countTotalEntries(resourceGroups map[string][]map[string]interface{}) int {
	total := 0
	for _, entries := range resourceGroups {
		total += len(entries)
	}
	return total
}

// HTTP handlers
func (e *Exporter) handleRoot(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "otlpeek - Look into otlp streams\n")
	fmt.Fprintf(w, "Last activity: %s\n", globalData.lastActivity.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "\nSummary:\n")
	fmt.Fprintf(w, "  Traces:  %d entries  (browse: /traces)\n", countTotalEntries(globalData.traces))
	fmt.Fprintf(w, "  Metrics: %d entries  (browse: /metrics)\n", countTotalEntries(globalData.metrics))
	fmt.Fprintf(w, "  Logs:    %d entries  (browse: /logs)\n", countTotalEntries(globalData.logs))
	fmt.Fprintf(w, "\nUse /traces, /metrics, or /logs to view raw gRPC messages in JSON format.\n")
}

func (e *Exporter) handleTraces(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := map[string]interface{}{
		"type":  "traces",
		"count": countTotalEntries(globalData.traces),
		"data":  globalData.traces,
	}

	json.NewEncoder(w).Encode(response)
}

func (e *Exporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := map[string]interface{}{
		"type":  "metrics",
		"count": countTotalEntries(globalData.metrics),
		"data":  globalData.metrics,
	}

	json.NewEncoder(w).Encode(response)
}

func (e *Exporter) handleLogs(w http.ResponseWriter, r *http.Request) {
	globalData.mu.RLock()
	defer globalData.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := map[string]interface{}{
		"type":  "logs",
		"count": countTotalEntries(globalData.logs),
		"data":  globalData.logs,
	}

	json.NewEncoder(w).Encode(response)
}
