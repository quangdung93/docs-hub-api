package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// TracerConfig là tham số khởi tạo tracer.
type TracerConfig struct {
	Enabled      bool
	ServiceName  string
	Environment  string
	OTLPEndpoint string  // host:port của OTLP gRPC collector
	SampleRatio  float64 // 0.0 - 1.0
}

// TracerProvider bọc SDK provider để bootstrap gọi Shutdown khi tắt.
type TracerProvider struct {
	provider *sdktrace.TracerProvider
}

// NewTracerProvider khởi tạo OpenTelemetry với OTLP gRPC exporter.
// Nếu Enabled=false, trả về provider no-op (Shutdown an toàn).
func NewTracerProvider(ctx context.Context, cfg TracerConfig) (*TracerProvider, error) {
	if !cfg.Enabled {
		return &TracerProvider{}, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("khởi tạo OTLP exporter thất bại: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("tạo resource OTel thất bại: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return &TracerProvider{provider: tp}, nil
}

// Shutdown flush và đóng tracer provider. An toàn khi provider nil (no-op).
func (t *TracerProvider) Shutdown(ctx context.Context) error {
	if t.provider == nil {
		return nil
	}
	if err := t.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracer provider: %w", err)
	}
	return nil
}
