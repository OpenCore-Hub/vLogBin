package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestSetupDisabledInstallsNoop(t *testing.T) {
	shutdown, err := Setup(Config{})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	// A no-op provider yields non-recording spans.
	_, span := otel.Tracer("test").Start(context.Background(), "x")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("expected no-op span while tracing disabled")
	}
}

func TestSetupNoopExporterKeepsDisabled(t *testing.T) {
	shutdown, err := Setup(Config{Enabled: true, Exporter: "noop"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "x")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("expected no-op span for explicit noop exporter")
	}
}

func TestSetupStdout(t *testing.T) {
	shutdown, err := Setup(Config{
		Enabled:     true,
		Exporter:    "stdout",
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSetupOTLPDoesNotDial(t *testing.T) {
	// Building the OTLP exporter must not require a live collector.
	shutdown, err := Setup(Config{
		Enabled:     true,
		Exporter:    "otlp",
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSetupOTLPWithEndpoint(t *testing.T) {
	shutdown, err := Setup(Config{
		Enabled:      true,
		Exporter:     "otlp",
		OTLPEndpoint: "collector.example:4318",
		ServiceName:  "test-service",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSetupUnknownExporter(t *testing.T) {
	if _, err := Setup(Config{Enabled: true, Exporter: "bogus"}); err == nil {
		t.Fatal("expected error for unknown exporter")
	}
}

func TestSetupInvalidSampleRatio(t *testing.T) {
	if _, err := Setup(Config{Enabled: true, Exporter: "noop", SampleRatio: 1.5}); err == nil {
		t.Fatal("expected error for sample ratio > 1")
	}
	if _, err := Setup(Config{Enabled: true, Exporter: "noop", SampleRatio: -0.1}); err == nil {
		t.Fatal("expected error for sample ratio < 0")
	}
}
