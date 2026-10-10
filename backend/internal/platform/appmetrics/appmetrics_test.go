package appmetrics_test

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
)

func collect(t *testing.T, r *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func TestNilMetricsRecordNothing(t *testing.T) {
	var m *appmetrics.Metrics
	ctx := context.Background()
	m.Ingested(ctx, appmetrics.IngestAccepted)
	m.Evaluated(ctx, time.Millisecond, "allow")
	m.RateLimited(ctx, appmetrics.LimitKey)
	m.LimiterUnavailable(ctx)
	m.AuthFailed(ctx)
	m.PreviousPepperUsed(ctx, true)
	m.Delivered(ctx, appmetrics.DeliverySucceeded, time.Millisecond)
	m.JobRan(ctx, "noop", appmetrics.JobSucceeded, time.Millisecond)
	if err := m.ObserveQueueDepth(nil); err != nil {
		t.Fatal(err)
	}
}

func TestRecordsByAttribute(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m, err := appmetrics.New(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m.Ingested(ctx, appmetrics.IngestAccepted)
	m.Ingested(ctx, appmetrics.IngestAccepted)
	m.Ingested(ctx, appmetrics.IngestRejected)
	m.RateLimited(ctx, appmetrics.LimitTenant)
	m.AuthFailed(ctx)
	m.Evaluated(ctx, 20*time.Millisecond, "allow")

	got := collect(t, reader)
	sum, ok := got["riskos.events.ingested"].Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("ingested is %T", got["riskos.events.ingested"].Data)
	}
	counts := map[string]int64{}
	for _, dp := range sum.DataPoints {
		v, _ := dp.Attributes.Value("result")
		counts[v.AsString()] = dp.Value
	}
	if counts["accepted"] != 2 || counts["rejected"] != 1 {
		t.Fatalf("ingested counts = %v", counts)
	}
	for _, name := range []string{"riskos.ratelimit.rejected", "riskos.auth.failures", "riskos.risk.evaluation.duration"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%s was not recorded", name)
		}
	}
}

func TestQueueDepthObserved(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m, err := appmetrics.New(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	err = m.ObserveQueueDepth(func(context.Context) ([]appmetrics.QueueDepth, error) {
		return []appmetrics.QueueDepth{{Queue: "webhooks", State: "available", Count: 7}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	g, ok := collect(t, reader)["riskos.jobs.queue.depth"].Data.(metricdata.Gauge[int64])
	if !ok || len(g.DataPoints) != 1 || g.DataPoints[0].Value != 7 {
		t.Fatalf("queue depth = %+v", g)
	}
}
