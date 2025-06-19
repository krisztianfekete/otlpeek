# otlpeek exporter

## Overview

`otlpeek` is an OpenTelemetry exporter that lets you peek into your `otlp` streams.

## Problem statement

Thanks to [various text exposition formats](https://prometheus.io/docs/instrumenting/exposition_formats/), you can test metrics easily, both manually and programatically. 

This is not quite the case with [otlp](https://opentelemetry.io/docs/specs/otlp/) data.

As of today, I could not find an [opentelemetry-contrib exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter) to solve this in a compact way.

Alternatives include deploying a proper tracing solution, but that might be a too much overhead in CI/CD pipelines, or for quick local testing.

## Features

`otlpeek` can be built into custom OpenTelemetry Collector distributions and when it's added to metrics, logs, or traces pipelines, it will a expose 
 - a high level overview at `/`
 - the last 100 entries by resource groups at `/metrics`, `/logs`, `/traces` in `JSON` format

Overview:

```
otlpeek - Look into otlp streams
Last activity: 2025-06-19 10:14:06

Summary:
  Traces:  12 entries  (browse: /traces)
  Metrics: 7 entries  (browse: /metrics)
  Logs:    5 entries  (browse: /logs)

Use /traces, /metrics, or /logs to view raw gRPC messages in JSON format.
```
Example `/traces` endpoint:

```json
{
  "count": 1,
  "data": {
    "map[service.name:simple-service-2]": [
      {
        "resource": {
          "attributes": {
            "service.name": "simple-service-2"
          }
        },
        "scopeSpans": [
          {
            "scope": {
              "name": "",
              "version": ""
            },
            "spans": [
              {
                "attributes": {
                  "http.method": "GET",
                  "http.route": "/api/users",
                  "http.status_code": 500
                },
                "endTimeUnixNano": 1664827201000000000,
                "kind": "Server",
                "name": "GET /api/users",
                "parentSpanId": "",
                "spanId": "",
                "startTimeUnixNano": 1664827200000000000,
                "status": {
                  "code": "Ok",
                  "message": ""
                },
                "traceId": ""
              }
            ]
          }
        ],
        "timestamp": "2025-06-19T14:24:50.22939118+02:00"
      }
    ]
  },
  "type": "traces"
}
```

## Trying it out

You can build the custom OpenTelemetry Collector with the otlpeek exporter using:

```
ocb --config builder-config.yaml
```

Then run it:

```
dist/otlpeek --config=cmd/config.yaml
```

Alternatively, you can also built it into a Docker image via this [Dockerfile](https://github.com/krisztianfekete/otlpeek/blob/main/Dockerfile).

You can also drop it into any Kubernetes cluster via this [example manifest](https://github.com/krisztianfekete/otlpeek/blob/main/k8s/otlpeek.yaml).
