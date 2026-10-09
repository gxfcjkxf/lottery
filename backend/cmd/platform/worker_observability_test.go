package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

func workerTestRuntime(t *testing.T, enabled bool) (*observability.Runtime, context.Context) {
	t.Helper()
	options := observability.Options{
		Service:     "worker",
		Environment: "test",
		Version:     "test",
		TraceRatio:  0.1,
	}
	if enabled {
		tokenPath := filepath.Join(t.TempDir(), "metrics.token")
		if err := os.WriteFile(tokenPath, []byte("0123456789abcdef0123456789abcdef"), 0600); err != nil {
			t.Fatal(err)
		}
		options.MetricsAddr = "127.0.0.1:19090"
		options.TokenFile = tokenPath
	}
	runtime, err := observability.New(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("runtime shutdown: %v", err)
		}
	})
	return runtime, observability.WithRuntime(context.Background(), runtime)
}

func workerMetricValue(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			matched := true
			for key, want := range labels {
				found := false
				for _, label := range metric.GetLabel() {
					if label.GetName() == key && label.GetValue() == want {
						found = true
						break
					}
				}
				if !found {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			if counter := metric.GetCounter(); counter != nil {
				return counter.GetValue(), true
			}
			if gauge := metric.GetGauge(); gauge != nil {
				return gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

func TestObservedWorkPreservesPartialCommitErrorWithoutRetry(t *testing.T) {
	runtime, ctx := workerTestRuntime(t, true)
	wantErr := errors.New("business operation failed after partial commit")
	calls := 0
	n, err := observedWork(ctx, "settlement", func(context.Context) (int, error) {
		calls++
		return 3, wantErr
	})
	if n != 3 || !errors.Is(err, wantErr) {
		t.Fatalf("observedWork returned (%d, %v), want (3, original error)", n, err)
	}
	if calls != 1 {
		t.Fatalf("work called %d times, want exactly once", calls)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_runs_total", map[string]string{"component": "settlement", "outcome": "error"}); !ok || got != 1 {
		t.Fatalf("error run metric = %v, present=%v; want 1", got, ok)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_committed_items_total", map[string]string{"component": "settlement"}); !ok || got != 3 {
		t.Fatalf("committed item metric = %v, present=%v; want 3", got, ok)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_inflight", map[string]string{"component": "settlement"}); !ok || got != 0 {
		t.Fatalf("inflight metric = %v, present=%v; want 0", got, ok)
	}
}

func TestObservedWorkRecordsSuccess(t *testing.T) {
	runtime, ctx := workerTestRuntime(t, true)
	calls := 0
	n, err := observedWork(ctx, "notification", func(context.Context) (int, error) {
		calls++
		return 5, nil
	})
	if n != 5 || err != nil || calls != 1 {
		t.Fatalf("observedWork returned (%d, %v) after %d calls; want (5, nil) after one call", n, err, calls)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_runs_total", map[string]string{"component": "notification", "outcome": "success"}); !ok || got != 1 {
		t.Fatalf("success run metric = %v, present=%v; want 1", got, ok)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_committed_items_total", map[string]string{"component": "notification"}); !ok || got != 5 {
		t.Fatalf("committed item metric = %v, present=%v; want 5", got, ok)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_inflight", map[string]string{"component": "notification"}); !ok || got != 0 {
		t.Fatalf("inflight metric = %v, present=%v; want 0", got, ok)
	}
}

func TestObservedWorkPropagatesPanicAndFinishesAsError(t *testing.T) {
	runtime, ctx := workerTestRuntime(t, true)
	payload := &struct{ message string }{message: "worker panic payload"}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		observedWork(ctx, "draw_correction", func(context.Context) (int, error) {
			panic(payload)
		})
	}()
	if recovered != payload {
		t.Fatalf("panic payload = %#v, want the original payload %#v", recovered, payload)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_runs_total", map[string]string{"component": "draw_correction", "outcome": "error"}); !ok || got != 1 {
		t.Fatalf("panic error run metric = %v, present=%v; want 1", got, ok)
	}
	if got, ok := workerMetricValue(t, runtime.Registry(), "lottery_worker_inflight", map[string]string{"component": "draw_correction"}); !ok || got != 0 {
		t.Fatalf("panic inflight metric = %v, present=%v; want 0", got, ok)
	}
}

func TestObservedWorkIsSideEffectFreeWhenTelemetryIsDisabled(t *testing.T) {
	runtime, ctx := workerTestRuntime(t, false)
	if runtime.Enabled() {
		t.Fatal("default runtime should be disabled")
	}
	calls := 0
	n, err := observedWork(ctx, "period_tick", func(context.Context) (int, error) {
		calls++
		return 2, nil
	})
	if n != 2 || err != nil || calls != 1 {
		t.Fatalf("observedWork returned (%d, %v) after %d calls; want (2, nil) after one call", n, err, calls)
	}
	if runtime.WorkerReady(time.Now()) {
		t.Fatal("disabled observation unexpectedly changed worker readiness")
	}
	families, err := runtime.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if len(family.GetMetric()) != 0 && family.GetName() != "lottery_build_info" {
			t.Fatalf("disabled observation materialized metric %q", family.GetName())
		}
	}
}
