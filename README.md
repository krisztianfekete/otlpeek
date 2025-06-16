# otlpeek exporter

## Build it into a Collector

You can build the custom OpenTelemetry Collector with the otlpeek exporter using:

```
ocb --config builder-config.yaml
```

Then run it:

```
dist/otlpeek --config=cmd/config.yaml
```

## Send a Sample Log Request

You can send a sample OTLP log request to the running collector using `grpcurl`:

```
grpcurl \
  -plaintext \
  -v \
  -d @ \
  -proto ../forks/opentelemetry-proto/opentelemetry/proto/logs/v1/logs.proto \
  -import-path ../forks/opentelemetry-proto \
  localhost:4317 \
  opentelemetry.proto.collector.logs.v1.LogsService/Export <<EOF
{
  "resourceLogs": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": { "stringValue": "simple-service" }
          }
        ]
      },
      "scopeLogs": [
        {
          "scope": {},
          "logRecords": [
            {
              "timeUnixNano": "1664827200000000000",
              "body": {
                "stringValue": "Sample log message"
              },
              "attributes": [
                {
                  "key": "log.file.name",
                  "value": { "stringValue": "app.log" }
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
EOF
```

- The `-proto` flag should point to the OTLP logs proto definition ([logs.proto](https://raw.githubusercontent.com/open-telemetry/opentelemetry-proto/refs/heads/main/opentelemetry/proto/logs/v1/logs.proto)).
- The `-import-path` should point to the root of your local clone of the OpenTelemetry proto repository.
- The collector must be running and listening on `localhost:4317` for this to work.
