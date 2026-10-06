package capacity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunCountsCallbackFailuresAndRequiresReceipt(t *testing.T) {
	plan := Plan{Rate: 4, Duration: 250 * time.Millisecond, MaxInFlight: 2, RequestTimeout: time.Second}
	report, err := Run(context.Background(), plan, func(context.Context, int) Observation {
		return Observation{Status: 201, Code: "bad payload", Err: "sensitive callback details"}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Planned != 1 || report.Started != 1 || report.Completed != 1 || report.Dropped != 0 || report.Succeeded != 0 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if report.ErrorCounts["callback_error"] != 1 || report.ErrorCounts["missing_order_id"] != 1 {
		t.Fatalf("error categories: %#v", report.ErrorCounts)
	}
	if report.CodeCounts["INVALID_CODE"] != 1 || len(report.CodeCounts) != 1 {
		t.Fatalf("malformed response code was not normalized: %#v", report.CodeCounts)
	}
	if strings.Contains(strings.Join(mapKeys(report.ErrorCounts), ","), "sensitive") || strings.Contains(strings.Join(mapKeys(report.CodeCounts), ","), "payload") {
		t.Fatalf("raw callback detail leaked: errors=%#v codes=%#v", report.ErrorCounts, report.CodeCounts)
	}
	if report.ServiceLatency.Count != 1 || report.StartLag.Count != 1 || report.EndToEnd.Count != 1 {
		t.Fatalf("latency samples must include started and completed request: %+v", report)
	}
}

func TestRunWaitsForScheduledWindowBeforeComputingThroughput(t *testing.T) {
	plan := Plan{Rate: 4, Duration: 250 * time.Millisecond, MaxInFlight: 1, RequestTimeout: time.Second}
	started := time.Now()
	report, err := Run(context.Background(), plan, func(context.Context, int) Observation {
		return Observation{Status: 201, OrderID: "order-1"}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if elapsed := time.Since(started); elapsed < plan.Duration {
		t.Fatalf("Run returned before scheduled window ended: elapsed=%s duration=%s", elapsed, plan.Duration)
	}
	if report.CompletionThroughputPerSecond > float64(plan.Rate) {
		t.Fatalf("fast callback inflated completion throughput: %+v", report)
	}
	if report.CodeCounts[""] != 1 || report.CodeCounts["INVALID_CODE"] != 0 {
		t.Fatal("absent success/error code was classified as malformed", report.CodeCounts)
	}
}

func TestRunCountsSuccessAfterScheduledInterval(t *testing.T) {
	plan := Plan{Rate: 4, Duration: 250 * time.Millisecond, MaxInFlight: 1, RequestTimeout: time.Second}
	report, err := Run(context.Background(), plan, func(context.Context, int) Observation {
		time.Sleep(275 * time.Millisecond)
		return Observation{Status: 201, OrderID: "order-1"}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Succeeded != 1 || report.SuccessesDuringScheduledInterval != 0 || report.SuccessesAfterScheduledInterval != 1 {
		t.Fatalf("scheduled-window success counts: %+v", report)
	}
	if report.CompletionThroughputPerSecond <= 0 || report.PlannedArrivalRatePerSecond != 4 {
		t.Fatalf("throughput distinctions missing: %+v", report)
	}
}

func TestRunDropsArrivalsAtInFlightLimit(t *testing.T) {
	plan := Plan{Rate: 100, Duration: 200 * time.Millisecond, MaxInFlight: 1, RequestTimeout: time.Second}
	report, err := Run(context.Background(), plan, func(context.Context, int) Observation {
		time.Sleep(100 * time.Millisecond)
		return Observation{Status: 201, OrderID: "order"}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Dropped == 0 || report.BackpressureDropped == 0 || report.PeakInFlight != 1 {
		t.Fatalf("expected bounded drops at one in-flight request: %+v", report)
	}
	if report.Planned != report.Started+report.Dropped || report.Completed != report.Started {
		t.Fatalf("count invariants failed: %+v", report)
	}
	if report.DropRate <= 0 || report.BackpressureDropRate <= 0 || report.FailureRate != 0 {
		t.Fatalf("drop/failure rates: %+v", report)
	}
}

func TestRunCancellationDrainsStartedCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		plan := Plan{Rate: 100, Duration: time.Second, MaxInFlight: 3, RequestTimeout: time.Second}
		report, err := Run(ctx, plan, func(requestCtx context.Context, _ int) Observation {
			started <- struct{}{}
			<-requestCtx.Done()
			return Observation{Err: requestCtx.Err().Error()}
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run error = %v, want context canceled", err)
		}
		if report.Completed != report.Started || report.Started == 0 || report.Planned != report.Started+report.Dropped {
			t.Errorf("canceled run did not drain/reconcile: %+v", report)
		}
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not drain canceled callback")
	}
}

func TestRunPassesRequestTimeoutContext(t *testing.T) {
	plan := Plan{Rate: 2, Duration: 500 * time.Millisecond, MaxInFlight: 1, RequestTimeout: 10 * time.Millisecond}
	report, err := Run(context.Background(), plan, func(ctx context.Context, _ int) Observation {
		<-ctx.Done()
		return Observation{Err: ctx.Err().Error()}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Completed != report.Started || report.ErrorCounts["callback_error"] != report.Started {
		t.Fatalf("request timeout was not propagated: %+v", report)
	}
}

func TestRunRejectsBadPlans(t *testing.T) {
	valid := Plan{Rate: 1, Duration: time.Second, MaxInFlight: 1, RequestTimeout: time.Second}
	tooLarge := Plan{Rate: 1_000_001, Duration: time.Second, MaxInFlight: 1, RequestTimeout: time.Second}
	for name, plan := range map[string]Plan{
		"zero rate":        {Duration: valid.Duration, MaxInFlight: 1, RequestTimeout: time.Second},
		"zero duration":    {Rate: 1, MaxInFlight: 1, RequestTimeout: time.Second},
		"zero concurrency": {Rate: 1, Duration: time.Second, RequestTimeout: time.Second},
		"zero timeout":     {Rate: 1, Duration: time.Second, MaxInFlight: 1},
		"too many":         tooLarge,
		"no arrivals":      {Rate: 1, Duration: time.Nanosecond, MaxInFlight: 1, RequestTimeout: time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			_, err := Run(context.Background(), plan, func(context.Context, int) Observation {
				called = true
				return Observation{}
			})
			if err == nil || called {
				t.Fatalf("Run(%+v) = %v, callback called=%v", plan, err, called)
			}
		})
	}
	if _, err := Run(context.Background(), valid, nil); err == nil {
		t.Fatal("nil callback accepted")
	}
}

func mapKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
