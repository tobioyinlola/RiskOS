// Package telemetry bootstraps OpenTelemetry tracing and metrics.
//
// Providers are returned to the caller and injected where needed; the process never mutates the
// OpenTelemetry globals. When telemetry is disabled, no-op providers keep instrumented code paths
// identical at near-zero cost.
package telemetry

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
)

// Providers bundles the telemetry providers for injection.
type Providers struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
	shutdown       []func(context.Context) error
}

// Setup creates providers for service in env according to cfg.
func Setup(ctx context.Context, cfg config.Telemetry, service string, env config.Environment) (*Providers, error) {
	p := &Providers{
		Propagator: propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}
	if !cfg.Enabled {
		p.TracerProvider = tracenoop.NewTracerProvider()
		p.MeterProvider = metricnoop.NewMeterProvider()
		return p, nil
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", service),
		attribute.String("deployment.environment.name", string(env)),
	)

	traceOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OTLPEndpoint)}
	metricOpts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.OTLPInsecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
		metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
	}

	traceExp, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)

	metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", errors.Join(err, tp.Shutdown(ctx)))
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)

	p.TracerProvider = tp
	p.MeterProvider = mp
	p.shutdown = []func(context.Context) error{tp.Shutdown, mp.Shutdown}
	return p, nil
}

// Shutdown flushes and stops exporters. It is safe to call on disabled providers.
func (p *Providers) Shutdown(ctx context.Context) error {
	var errs []error
	for _, fn := range p.shutdown {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
