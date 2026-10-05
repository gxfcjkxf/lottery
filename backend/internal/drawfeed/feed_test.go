package drawfeed

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

type adapterFunc func(context.Context, Source, Request) (Candidate, error)

func TestResolveClaimGuardStopsFallbackWithoutErasingEvidence(t *testing.T) {
	var fetched, guarded int
	stop := errors.New("manual selection superseded claim")
	r := DefaultResolver(time.Second)
	r.Adapters["api"] = adapterFunc(func(context.Context, Source, Request) (Candidate, error) {
		fetched++
		return Candidate{}, ErrNoData
	})
	r.BeforeAttempt = func(context.Context, Source) error {
		guarded++
		if guarded == 2 {
			return stop
		}
		return nil
	}
	out, err := r.Resolve(context.Background(), Request{PeriodNo: "p1", Model: feedDigitsModel()}, []Source{
		{ID: "primary", Type: "api", Priority: 1, Enabled: true},
		{ID: "backup", Type: "api", Priority: 2, Enabled: true},
	})
	if !errors.Is(err, stop) || fetched != 1 || guarded != 2 || len(out.Attempts) != 1 || out.Attempts[0].SourceID != "primary" {
		t.Fatalf("claim guard: calls=%d guarded=%d result=%+v err=%v", fetched, guarded, out, err)
	}
}

func TestResolvePeriodCandidateCheckFallsBackAndStopsInfrastructureErrors(t *testing.T) {
	for _, infrastructure := range []bool{false, true} {
		calls := 0
		r := DefaultResolver(time.Second)
		r.Adapters["api"] = adapterFunc(func(context.Context, Source, Request) (Candidate, error) {
			calls++
			return candidate("p1", rules.Draw{Digits: []int{1, 2, 3}}), nil
		})
		broken := errors.New("primary clock unavailable")
		r.CandidateCheck = func(context.Context, Candidate) error {
			if infrastructure {
				return broken
			}
			if calls == 1 {
				return ErrInvalid
			}
			return nil
		}
		out, err := r.Resolve(context.Background(), Request{PeriodNo: "p1", Model: feedDigitsModel()}, []Source{{ID: "primary", Type: "api", Priority: 1, Enabled: true}, {ID: "backup", Type: "api", Priority: 2, Enabled: true}})
		if infrastructure {
			if !errors.Is(err, broken) || calls != 1 || len(out.Attempts) != 1 || out.Attempts[0].Code != "candidate_check_error" {
				t.Fatalf("infrastructure=%+v %v calls=%d", out, err, calls)
			}
		} else if err != nil || calls != 2 || out.SourceID != "backup" || out.Attempts[0].Status != "abnormal" {
			t.Fatalf("fallback=%+v %v calls=%d", out, err, calls)
		}
	}
}

func (f adapterFunc) Fetch(ctx context.Context, source Source, request Request) (Candidate, error) {
	return f(ctx, source, request)
}

func feedXModel() rules.Model {
	return rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 10}, RegularCount: 2, SpecialCount: 1}
}

func feedMModel() rules.Model {
	return rules.Model{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 2, RegularCount: 1, SpecialCount: 1, RegularPool: rules.Pool{Min: 1, Max: 5}, SpecialPool: rules.Pool{Min: 1, Max: 5}}
}

func feedDigitsModel() rules.Model {
	return rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true, AllowRepeat: true}
}

func candidate(period string, draw rules.Draw) Candidate {
	return Candidate{PeriodNo: period, Draw: draw, DrawnAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)), RawReference: "record-1"}
}

func TestValidateCandidateAcceptsEachModelAndRejectsMalformedResults(t *testing.T) {
	tests := []struct {
		name      string
		model     rules.Model
		candidate Candidate
		wantErr   error
	}{
		{"X plus Y", feedXModel(), candidate("20261006", rules.Draw{Regular: []int{4, 12}, Special: []int{7}}), nil},
		{"M select N", feedMModel(), candidate("20261006", rules.Draw{Regular: []int{2}, Special: []int{5}}), nil},
		{"ordered digits including zero", feedDigitsModel(), candidate("20261006", rules.Draw{Digits: []int{0, 1, 0}}), nil},
		{"period mismatch", feedXModel(), candidate("other", rules.Draw{Regular: []int{4, 12}, Special: []int{7}}), ErrInvalid},
		{"missing period", feedXModel(), candidate(" ", rules.Draw{Regular: []int{4, 12}, Special: []int{7}}), ErrInvalid},
		{"oversized period", feedXModel(), candidate(string(make([]byte, 81)), rules.Draw{Regular: []int{4, 12}, Special: []int{7}}), ErrInvalid},
		{"wrong count", feedXModel(), candidate("20261006", rules.Draw{Regular: []int{4}, Special: []int{7}}), ErrInvalid},
		{"out of range", feedXModel(), candidate("20261006", rules.Draw{Regular: []int{4, 50}, Special: []int{7}}), ErrInvalid},
		{"repeated number", feedXModel(), candidate("20261006", rules.Draw{Regular: []int{4, 4}, Special: []int{7}}), ErrInvalid},
		{"M cross pool repeat", feedMModel(), candidate("20261006", rules.Draw{Regular: []int{2}, Special: []int{2}}), ErrInvalid},
		{"irrelevant groups", feedDigitsModel(), candidate("20261006", rules.Draw{Regular: []int{1}, Digits: []int{1, 2, 3}}), ErrInvalid},
		{"zero timestamp", feedXModel(), Candidate{PeriodNo: "20261006", Draw: rules.Draw{Regular: []int{4, 12}, Special: []int{7}}}, ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCandidate(Request{PeriodNo: "20261006", Model: test.model}, test.candidate)
			if test.wantErr == nil && err != nil {
				t.Fatalf("ValidateCandidate() error = %v", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateCandidate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateCandidateRejectsSamePreviousDrawAfterCanonicalization(t *testing.T) {
	x := feedXModel()
	request := Request{PeriodNo: "p2", Model: x, Previous: &rules.Draw{Regular: []int{2, 1}, Special: []int{3}}}
	if err := ValidateCandidate(request, candidate("p2", rules.Draw{Regular: []int{1, 2}, Special: []int{3}})); !errors.Is(err, ErrAbnormal) {
		t.Fatalf("unordered equivalent draw error = %v, want ErrAbnormal", err)
	}
	request.Model = feedDigitsModel()
	request.Previous = &rules.Draw{Digits: []int{1, 2, 3}}
	if err := ValidateCandidate(request, candidate("p2", rules.Draw{Digits: []int{3, 2, 1}})); err != nil {
		t.Fatalf("reordered ordered digits were treated as equal: %v", err)
	}
	request.Previous = nil
	if err := ValidateCandidate(request, candidate("p2", rules.Draw{Digits: []int{1, 2, 3}})); err != nil {
		t.Fatalf("initial draw with no previous result rejected: %v", err)
	}
}

func TestResolvePriorityFailoverFirstValidAndAttemptStatuses(t *testing.T) {
	request := Request{PeriodNo: "p1", Model: feedDigitsModel()}
	var calls []string
	resolver := Resolver{Timeout: time.Second, Adapters: map[string]Adapter{
		"api": adapterFunc(func(_ context.Context, source Source, _ Request) (Candidate, error) {
			calls = append(calls, source.ID)
			switch source.ID {
			case "late":
				return candidate("p1", rules.Draw{Digits: []int{9, 8, 7}}), nil
			case "bad":
				return candidate("p1", rules.Draw{Digits: []int{10, 8, 7}}), nil
			default:
				return Candidate{}, ErrNoData
			}
		}),
		"dom": adapterFunc(func(_ context.Context, source Source, _ Request) (Candidate, error) {
			calls = append(calls, source.ID)
			return Candidate{}, ErrTimeout
		}),
	}}
	sources := []Source{
		{ID: "late", Type: "api", Priority: 5, Enabled: true},
		{ID: "manual", Type: "manual", Priority: 1, Enabled: true},
		{ID: "disabled", Type: "api", Priority: 2},
		{ID: "empty", Type: "api", Priority: 3, Enabled: true},
		{ID: "timeout", Type: "dom", Priority: 4, Enabled: true},
		{ID: "bad", Type: "api", Priority: 6, Enabled: true},
	}
	got, err := resolver.Resolve(context.Background(), request, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.SourceID != "late" || !reflect.DeepEqual(calls, []string{"empty", "timeout", "late"}) {
		t.Fatalf("source winner/call order = %q/%v", got.SourceID, calls)
	}
	if got.Candidate.DrawnAt.Location() != time.UTC {
		t.Fatalf("candidate timestamp location = %v, want UTC", got.Candidate.DrawnAt.Location())
	}
	wantAttempts := []Attempt{
		{SourceID: "empty", Status: "no_data", Code: "no_data"},
		{SourceID: "timeout", Status: "timeout", Code: "timeout"},
		{SourceID: "late", Status: "success", Code: "ok"},
	}
	if !reflect.DeepEqual(got.Attempts, wantAttempts) {
		t.Fatalf("attempts = %+v, want %+v", got.Attempts, wantAttempts)
	}
}

func TestResolveAbnormalThenNoDataReturnsLastFailureAndSkipsUnavailableAdapter(t *testing.T) {
	request := Request{PeriodNo: "p1", Model: feedDigitsModel()}
	resolver := Resolver{Timeout: time.Second, Adapters: map[string]Adapter{
		"api": adapterFunc(func(context.Context, Source, Request) (Candidate, error) {
			return Candidate{}, errors.New("source failure")
		}),
	}}
	got, err := resolver.Resolve(context.Background(), request, []Source{
		{ID: "api", Type: "api", Priority: 1, Enabled: true},
		{ID: "missing", Type: "dom", Priority: 2, Enabled: true},
	})
	if !errors.Is(err, ErrAbnormal) || len(got.Attempts) != 2 || got.Attempts[0].Status != "error" || got.Attempts[1].Code != "adapter_unavailable" {
		t.Fatalf("Resolve() = %+v, %v", got, err)
	}
}

func TestResolveTimesOutEachAttemptAndContinues(t *testing.T) {
	request := Request{PeriodNo: "p1", Model: feedDigitsModel()}
	calls := 0
	resolver := Resolver{Timeout: 10 * time.Millisecond, Adapters: map[string]Adapter{
		"api": adapterFunc(func(ctx context.Context, _ Source, _ Request) (Candidate, error) {
			calls++
			<-ctx.Done()
			return Candidate{}, ctx.Err()
		}),
	}}
	got, err := resolver.Resolve(context.Background(), request, []Source{
		{ID: "one", Type: "api", Priority: 1, Enabled: true},
		{ID: "two", Type: "api", Priority: 2, Enabled: true},
	})
	if !errors.Is(err, ErrTimeout) || calls != 2 || len(got.Attempts) != 2 || got.Attempts[0].Status != "timeout" {
		t.Fatalf("Resolve() = %+v, %v, calls=%d", got, err, calls)
	}
}

func TestResolveHonorsParentCancellationWithoutFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	resolver := Resolver{Timeout: time.Second, Adapters: map[string]Adapter{
		"api": adapterFunc(func(ctx context.Context, _ Source, _ Request) (Candidate, error) {
			calls++
			cancel()
			<-ctx.Done()
			return Candidate{}, ctx.Err()
		}),
	}}
	got, err := resolver.Resolve(ctx, Request{PeriodNo: "p1", Model: feedDigitsModel()}, []Source{
		{ID: "one", Type: "api", Priority: 1, Enabled: true},
		{ID: "two", Type: "api", Priority: 2, Enabled: true},
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || len(got.Attempts) != 1 || got.Attempts[0].Code != "cancelled" {
		t.Fatalf("Resolve() = %+v, %v, calls=%d", got, err, calls)
	}
}

func TestResolveValidatesSourceConfigurationAndEmptyData(t *testing.T) {
	request := Request{PeriodNo: "p1", Model: feedDigitsModel()}
	resolver := DefaultResolver(time.Second)
	got, err := resolver.Resolve(context.Background(), request, []Source{{ID: "api", Type: "api", Priority: 1, Enabled: true}})
	if !errors.Is(err, ErrNoData) || len(got.Attempts) != 1 || got.Attempts[0].Status != "no_data" {
		t.Fatalf("stub result = %+v, %v", got, err)
	}
	badConfigs := [][]Source{
		{{ID: "a", Type: "api", Priority: 0}},
		{{ID: "a", Type: "api", Priority: 1}, {ID: "b", Type: "dom", Priority: 1}},
		{{ID: "a", Type: "manual", Priority: 1}, {ID: "a", Type: "api", Priority: 2}},
		{{ID: "a", Type: "http", Priority: 1}},
	}
	for i, sources := range badConfigs {
		if _, err := resolver.Resolve(context.Background(), request, sources); !errors.Is(err, ErrInvalid) {
			t.Errorf("bad source config %d error = %v, want ErrInvalid", i, err)
		}
	}
	tooMany := make([]Source, maxSourceCount+1)
	for i := range tooMany {
		tooMany[i] = Source{ID: string(rune('a' + i)), Type: "api", Priority: i + 1}
	}
	if _, err := resolver.Resolve(context.Background(), request, tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many sources error = %v, want ErrInvalid", err)
	}
}

func TestDefaultResolverStubsAPIAndDOMWithoutNetwork(t *testing.T) {
	resolver := DefaultResolver(time.Second)
	for _, sourceType := range []string{"api", "dom"} {
		got, err := resolver.Resolve(context.Background(), Request{PeriodNo: "p1", Model: feedDigitsModel()}, []Source{{ID: sourceType, Type: sourceType, Priority: 1, Enabled: true}})
		if !errors.Is(err, ErrNoData) || len(got.Attempts) != 1 || got.Attempts[0].Status != "no_data" {
			t.Errorf("%s stub = %+v, %v", sourceType, got, err)
		}
	}
}
