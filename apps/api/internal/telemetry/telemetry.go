// Package telemetry bootstraps OpenTelemetry distributed tracing: a global
// tracer provider, span processors (OTLP HTTP by default, stdout for local
// debugging) and the W3C Trace-Context/Baggage propagators. Instrumentation in
// httpapi/webhook/billing/store resolves its tracer through the global
// provider, so when tracing is disabled every span is a no-op and the service
// pays negligible overhead.
//
// Tracing is opt-in: set Enabled in the config (OTEL_ENABLED=true) and point
// OTEL_EXPORTER_OTLP_ENDPOINT at a collector. Deployments without a collector
// run unchanged with the no-op tracer.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config controls the global tracer provider. The zero value keeps tracing
// disabled (the no-op provider), which is the safe default for environments
// without a collector.
type Config struct {
	// Enabled turns distributed tracing on. When false (default), Setup
	// installs a no-op provider and returns a no-op shutdown func.
	Enabled bool
	// Exporter selects the span exporter: "otlp" (default), "stdout" (local
	// debugging) or "noop" (explicitly off). Unknown values are rejected.
	Exporter string
	// OTLPEndpoint is the collector base URL, e.g. "otel-collector:4318".
	// Empty uses the exporter default (http://localhost:4318) plus any
	// OTEL_EXPORTER_OTLP_ENDPOINT env var the exporter reads itself.
	OTLPEndpoint string
	// ServiceName is reported as the service.name resource attribute.
	ServiceName string
	// Environment is reported as deployment.environment (dev/staging/prod).
	Environment string
	// SampleRatio in [0,1]: 1 samples every trace (default), 0 nothing,
	// fractions use head sampling by trace ID.
	SampleRatio float64
	// BatchTimeout flushes a batch of spans after this idle window.
	BatchTimeout time.Duration
	// ExportTimeout caps a single export attempt.
	ExportTimeout time.Duration
	// MaxQueueSize bounds the in-memory span queue per exporter.
	MaxQueueSize int
	// MaxExportBatchSize bounds spans per export batch.
	MaxExportBatchSize int
}

// Setup installs the global tracer provider and W3C propagators and returns a
// shutdown func (safe to call when disabled). Call shutdown exactly once on
// process exit to flush pending spans.
func Setup(cfg Config) (func(context.Context) error, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "vlogbin-api"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("telemetry: sample ratio %v out of range [0,1]", cfg.SampleRatio)
	}

	if !cfg.Enabled || cfg.Exporter == "noop" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{}))
		return func(context.Context) error { return nil }, nil
	}

	exp, err := newSpanExporter(cfg)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironmentName(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	var sampler sdktrace.Sampler = sdktrace.AlwaysSample()
	switch {
	case cfg.SampleRatio <= 0:
		sampler = sdktrace.NeverSample()
	case cfg.SampleRatio < 1:
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRatio)
	}

	batchOpts := make([]sdktrace.BatchSpanProcessorOption, 0, 4)
	if cfg.BatchTimeout > 0 {
		batchOpts = append(batchOpts, sdktrace.WithBatchTimeout(cfg.BatchTimeout))
	}
	if cfg.ExportTimeout > 0 {
		batchOpts = append(batchOpts, sdktrace.WithExportTimeout(cfg.ExportTimeout))
	}
	if cfg.MaxQueueSize > 0 {
		batchOpts = append(batchOpts, sdktrace.WithMaxQueueSize(cfg.MaxQueueSize))
	}
	if cfg.MaxExportBatchSize > 0 {
		batchOpts = append(batchOpts, sdktrace.WithMaxExportBatchSize(cfg.MaxExportBatchSize))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, batchOpts...),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}

func newSpanExporter(cfg Config) (sdktrace.SpanExporter, error) {
	switch cfg.Exporter {
	case "", "otlp":
		opts := make([]otlptracehttp.Option, 0, 1)
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		exp, err := otlptracehttp.New(context.Background(), opts...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: create OTLP exporter: %w", err)
		}
		return exp, nil
	case "stdout":
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("telemetry: create stdout exporter: %w", err)
		}
		return exp, nil
	default:
		return nil, fmt.Errorf("telemetry: unknown exporter %q (want otlp|stdout|noop)", cfg.Exporter)
	}
}
