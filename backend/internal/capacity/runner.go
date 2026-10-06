// Package capacity provides a bounded, open-loop request load driver.
package capacity

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"
)

const maxPlannedRequests = 1_000_000
const maxInFlightLimit = 10_000

// Plan defines a constant-rate run. Arrivals are scheduled independently of
// request completions, with no more than MaxInFlight callbacks active at once.
type Plan struct {
	Rate           int           `json:"rate"`
	Duration       time.Duration `json:"duration"`
	MaxInFlight    int           `json:"max_in_flight"`
	RequestTimeout time.Duration `json:"request_timeout"`
}

// Observation is the callback's sanitized, structured view of one response.
// Err is used only to count callback failures and is never copied into Report.
type Observation struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	OrderID string `json:"order_id"`
	Err     string `json:"err"`
}

// LatencySummary describes a distribution in milliseconds.
type LatencySummary struct {
	Count int     `json:"count"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

// Report contains counts and latency distributions for a completed run.
type Report struct {
	Planned                          int            `json:"planned"`
	Started                          int            `json:"started"`
	Completed                        int            `json:"completed"`
	Dropped                          int            `json:"dropped"`
	BackpressureDropped              int            `json:"backpressure_dropped"`
	Succeeded                        int            `json:"succeeded"`
	StatusCounts                     map[int]int    `json:"status_counts"`
	CodeCounts                       map[string]int `json:"code_counts"`
	ErrorCounts                      map[string]int `json:"error_counts"`
	ServiceLatency                   LatencySummary `json:"service_latency"`
	StartLag                         LatencySummary `json:"start_lag"`
	EndToEnd                         LatencySummary `json:"end_to_end"`
	PeakInFlight                     int            `json:"peak_in_flight"`
	SuccessesDuringScheduledInterval int            `json:"successes_during_scheduled_interval"`
	SuccessesAfterScheduledInterval  int            `json:"successes_after_scheduled_interval"`
	WallTime                         time.Duration  `json:"wall_time"`
	CompletionThroughputPerSecond    float64        `json:"completion_throughput_per_second"`
	PlannedArrivalRatePerSecond      float64        `json:"planned_arrival_rate_per_second"`
	StartedArrivalRatePerSecond      float64        `json:"started_arrival_rate_per_second"`
	DropRate                         float64        `json:"drop_rate"`
	BackpressureDropRate             float64        `json:"backpressure_drop_rate"`
	FailureRate                      float64        `json:"failure_rate"`
}

// Run schedules planned arrivals at absolute offsets from the run start.
// When the in-flight limit is reached, that arrival is dropped immediately.
// On cancellation, no new arrivals are scheduled and all started callbacks
// are drained before Run returns. Callbacks should honor their request context;
// a callback that ignores cancellation can delay Run indefinitely.
func Run(ctx context.Context, plan Plan, callback func(context.Context, int) Observation) (Report, error) {
	report := emptyReport()
	planned, err := validatePlan(ctx, plan, callback)
	if err != nil {
		return report, err
	}
	report.Planned = planned
	start := time.Now()
	scheduledEnd := start.Add(plan.Duration)
	maxInFlight := min(plan.MaxInFlight, planned, maxInFlightLimit)
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var serviceSamples, lagSamples, endToEndSamples []float64
	inFlight := 0

	for i := 0; i < planned; i++ {
		target := start.Add(time.Duration(int64(i) * int64(time.Second) / int64(plan.Rate)))
		if err := waitUntil(ctx, target); err != nil {
			break
		}
		select {
		case sem <- struct{}{}:
			actualStart := time.Now()
			lag := actualStart.Sub(target)
			mu.Lock()
			inFlight++
			report.Started++
			if inFlight > report.PeakInFlight {
				report.PeakInFlight = inFlight
			}
			mu.Unlock()
			wg.Add(1)
			go func(index int, began time.Time, startLag time.Duration, target time.Time) {
				defer wg.Done()
				defer func() { <-sem }()
				requestCtx, cancel := context.WithTimeout(ctx, plan.RequestTimeout)
				observation := callback(requestCtx, index)
				cancel()
				finished := time.Now()
				service := finished.Sub(began)
				endToEnd := finished.Sub(target)
				mu.Lock()
				defer mu.Unlock()
				inFlight--
				report.Completed++
				report.StatusCounts[observation.Status]++
				code := observation.Code
				if code != "" && !validCode(code) {
					code = "INVALID_CODE"
				}
				report.CodeCounts[code]++
				if observation.Err != "" {
					report.ErrorCounts["callback_error"]++
				}
				if observation.Status != 201 {
					report.ErrorCounts["http_status"]++
				}
				if observation.Code != "" {
					report.ErrorCounts["response_code"]++
				}
				if observation.OrderID == "" {
					report.ErrorCounts["missing_order_id"]++
				}
				if observation.Status == 201 && observation.Code == "" && observation.OrderID != "" && observation.Err == "" {
					report.Succeeded++
					if finished.Before(scheduledEnd) {
						report.SuccessesDuringScheduledInterval++
					} else {
						report.SuccessesAfterScheduledInterval++
					}
				}
				serviceSamples = append(serviceSamples, float64(service)/float64(time.Millisecond))
				lagSamples = append(lagSamples, float64(startLag)/float64(time.Millisecond))
				endToEndSamples = append(endToEndSamples, float64(endToEnd)/float64(time.Millisecond))
			}(i, actualStart, lag, target)
		default:
			report.Dropped++
			report.BackpressureDropped++
		}
	}
	// Keep the load window open through its planned end, even when the final
	// arrival and all of its callbacks finish early.
	_ = waitUntil(ctx, scheduledEnd)
	// Cancellation prevents future arrivals; account for those planned slots as
	// dropped so planned always reconciles with started plus dropped.
	if remaining := planned - report.Started - report.Dropped; remaining > 0 {
		report.Dropped += remaining
	}
	wg.Wait()
	finished := time.Now()
	report.WallTime = finished.Sub(start)
	report.ServiceLatency = summarize(serviceSamples)
	report.StartLag = summarize(lagSamples)
	report.EndToEnd = summarize(endToEndSamples)
	if report.WallTime > 0 {
		report.CompletionThroughputPerSecond = float64(report.Completed) / report.WallTime.Seconds()
	}
	report.PlannedArrivalRatePerSecond = float64(report.Planned) / plan.Duration.Seconds()
	report.StartedArrivalRatePerSecond = float64(report.Started) / plan.Duration.Seconds()
	if report.Planned > 0 {
		report.DropRate = float64(report.Dropped) / float64(report.Planned)
		report.BackpressureDropRate = float64(report.BackpressureDropped) / float64(report.Planned)
	}
	if report.Completed > 0 {
		report.FailureRate = float64(report.Completed-report.Succeeded) / float64(report.Completed)
	}
	if report.Started+report.Dropped != report.Planned {
		return report, errors.New("capacity: internal arrival count invariant violated")
	}
	if report.Completed != report.Started || report.Succeeded != report.SuccessesDuringScheduledInterval+report.SuccessesAfterScheduledInterval {
		return report, errors.New("capacity: internal completion count invariant violated")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func validatePlan(ctx context.Context, p Plan, callback func(context.Context, int) Observation) (int, error) {
	if ctx == nil || callback == nil || p.Rate <= 0 || p.Duration <= 0 || p.MaxInFlight <= 0 || p.RequestTimeout <= 0 {
		return 0, errors.New("capacity: invalid plan, context, or callback")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int64(p.Rate) > math.MaxInt64/int64(p.Duration) {
		return 0, errors.New("capacity: planned request count overflows")
	}
	count := int64(p.Rate) * int64(p.Duration) / int64(time.Second)
	if count <= 0 || count > maxPlannedRequests {
		return 0, errors.New("capacity: planned request count must be between 1 and 1000000")
	}
	return int(count), nil
}

func waitUntil(ctx context.Context, target time.Time) error {
	delay := time.Until(target)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func emptyReport() Report {
	return Report{
		StatusCounts: make(map[int]int), CodeCounts: make(map[string]int), ErrorCounts: make(map[string]int),
	}
}

func summarize(values []float64) LatencySummary {
	if len(values) == 0 {
		return LatencySummary{}
	}
	sort.Float64s(values)
	return LatencySummary{
		Count: len(values), P50Ms: percentile(values, .50), P95Ms: percentile(values, .95), P99Ms: percentile(values, .99),
	}
}

func percentile(sorted []float64, p float64) float64 {
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func validCode(code string) bool {
	if len(code) == 0 || len(code) > 80 || code[0] < 'A' || code[0] > 'Z' {
		return false
	}
	for i := 1; i < len(code); i++ {
		c := code[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}
