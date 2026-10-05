// Package drawfeed validates externally supplied draw results and tries a
// bounded, ordered list of configured sources. It performs no network access.
package drawfeed

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

var (
	ErrInvalid  = errors.New("invalid draw feed input")
	ErrNoData   = errors.New("draw feed has no data")
	ErrTimeout  = errors.New("draw feed source timed out")
	ErrAbnormal = errors.New("draw feed result is abnormal")
)

const (
	maxPeriodBytes = 80
	maxSourceCount = 16
	maxSourceID    = 80
	maxRawRefBytes = 2048
)

type Request struct {
	PeriodNo string
	Model    rules.Model
	Previous *rules.Draw
}

type Candidate struct {
	PeriodNo     string     `json:"period_no"`
	Draw         rules.Draw `json:"result"`
	DrawnAt      time.Time  `json:"drawn_at"`
	RawReference string     `json:"raw_reference"`
}

type Source struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

type Attempt struct {
	SourceID string `json:"source_id"`
	Status   string `json:"status"`
	Code     string `json:"code"`
}

type Result struct {
	Candidate Candidate `json:"candidate"`
	SourceID  string    `json:"source_id"`
	Attempts  []Attempt `json:"attempts"`
}

type Adapter interface {
	// Fetch implementations must honor ctx cancellation and bound their I/O/work.
	// Resolver timeouts are cooperative: Fetch runs inline and is not force-stopped
	// by an unbounded goroutine when its context expires.
	Fetch(context.Context, Source, Request) (Candidate, error)
}

type Resolver struct {
	Adapters map[string]Adapter
	Timeout  time.Duration
}

// StubAdapter explicitly represents an API or DOM source that is not
// implemented. It never performs HTTP, browser, or other external I/O.
type StubAdapter struct{}

func (StubAdapter) Fetch(ctx context.Context, _ Source, _ Request) (Candidate, error) {
	if ctx == nil {
		return Candidate{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}
	return Candidate{}, ErrNoData
}

// DefaultResolver installs explicit no-network adapters for the future api
// and dom source types. A nonpositive timeout selects a conservative default.
func DefaultResolver(timeout time.Duration) Resolver {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return Resolver{
		Adapters: map[string]Adapter{"api": StubAdapter{}, "dom": StubAdapter{}},
		Timeout:  timeout,
	}
}

// ValidateCandidate checks that a feed result is a well-formed draw for the
// requested period and model, and rejects an unchanged repeat of the prior
// draw. Unordered number groups are compared after sorting; digit positions
// remain ordered.
func ValidateCandidate(request Request, candidate Candidate) error {
	if !validPeriod(request.PeriodNo) || !validPeriod(candidate.PeriodNo) || candidate.PeriodNo != request.PeriodNo {
		return ErrInvalid
	}
	if len(candidate.RawReference) > maxRawRefBytes || !utf8.ValidString(candidate.RawReference) {
		return ErrInvalid
	}
	if candidate.DrawnAt.IsZero() {
		return ErrInvalid
	}
	if err := rules.ValidateDraw(request.Model, candidate.Draw); err != nil {
		return ErrInvalid
	}
	if request.Previous != nil {
		if err := rules.ValidateDraw(request.Model, *request.Previous); err != nil {
			return ErrInvalid
		}
		if sameDraw(request.Model, *request.Previous, candidate.Draw) {
			return ErrAbnormal
		}
	}
	return nil
}

// Resolve tries enabled api/dom sources in ascending priority order. Priorities
// must be unique positive integers across the supplied source configuration.
// Each source gets its own timeout, bounded by the caller's context.
func (r Resolver) Resolve(ctx context.Context, request Request, sources []Source) (Result, error) {
	result := Result{Attempts: []Attempt{}}
	if ctx == nil || r.Timeout <= 0 || !validPeriod(request.PeriodNo) || rules.ValidateModel(request.Model) != nil || len(sources) > maxSourceCount {
		return result, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	ordered := slices.Clone(sources)
	seenIDs := make(map[string]struct{}, len(ordered))
	seenPriorities := make(map[int]struct{}, len(ordered))
	for _, source := range ordered {
		if !validIdentifier(source.ID) || source.Priority <= 0 {
			return result, ErrInvalid
		}
		if _, ok := seenIDs[source.ID]; ok {
			return result, ErrInvalid
		}
		if _, ok := seenPriorities[source.Priority]; ok {
			return result, ErrInvalid
		}
		seenIDs[source.ID] = struct{}{}
		seenPriorities[source.Priority] = struct{}{}
		if source.Type != "api" && source.Type != "dom" && source.Type != "manual" {
			return result, ErrInvalid
		}
	}
	slices.SortFunc(ordered, func(a, b Source) int {
		if a.Priority < b.Priority {
			return -1
		}
		if a.Priority > b.Priority {
			return 1
		}
		return 0
	})

	lastErr := error(ErrNoData)
	for _, source := range ordered {
		if !source.Enabled || source.Type == "manual" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		adapter := r.Adapters[source.Type]
		if adapter == nil {
			result.Attempts = append(result.Attempts, Attempt{SourceID: source.ID, Status: "error", Code: "adapter_unavailable"})
			lastErr = ErrAbnormal
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, r.Timeout)
		candidate, fetchErr := adapter.Fetch(attemptCtx, source, request)
		attemptErr := attemptCtx.Err()
		cancel()
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if errors.Is(attemptErr, context.DeadlineExceeded) {
			result.Attempts = append(result.Attempts, Attempt{SourceID: source.ID, Status: "timeout", Code: "timeout"})
			lastErr = ErrTimeout
			continue
		}
		if errors.Is(attemptErr, context.Canceled) {
			return result, context.Canceled
		}
		if fetchErr != nil {
			status, code, mapped := classifyError(fetchErr)
			result.Attempts = append(result.Attempts, Attempt{SourceID: source.ID, Status: status, Code: code})
			lastErr = mapped
			continue
		}
		if err := ValidateCandidate(request, candidate); err != nil {
			result.Attempts = append(result.Attempts, Attempt{SourceID: source.ID, Status: "abnormal", Code: "invalid_candidate"})
			lastErr = ErrAbnormal
			continue
		}
		candidate.DrawnAt = candidate.DrawnAt.UTC()
		result.Candidate = candidate
		result.SourceID = source.ID
		result.Attempts = append(result.Attempts, Attempt{SourceID: source.ID, Status: "success", Code: "ok"})
		return result, nil
	}
	return result, lastErr
}

func classifyError(err error) (status, code string, mapped error) {
	switch {
	case errors.Is(err, ErrNoData):
		return "no_data", "no_data", ErrNoData
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout", "timeout", ErrTimeout
	case errors.Is(err, ErrAbnormal), errors.Is(err, ErrInvalid):
		return "abnormal", "abnormal", ErrAbnormal
	default:
		return "error", "fetch_error", ErrAbnormal
	}
}

func validPeriod(value string) bool {
	return value != "" && strings.TrimSpace(value) != "" && len(value) <= maxPeriodBytes && utf8.ValidString(value)
}

func validIdentifier(value string) bool {
	return value != "" && strings.TrimSpace(value) != "" && len(value) <= maxSourceID && utf8.ValidString(value)
}

func sameDraw(model rules.Model, previous, current rules.Draw) bool {
	previous = cloneDraw(previous)
	current = cloneDraw(current)
	if model.Type != "DIGITS_0_9" && !model.Ordered {
		slices.Sort(previous.Regular)
		slices.Sort(current.Regular)
		slices.Sort(previous.Special)
		slices.Sort(current.Special)
	}
	return slices.Equal(previous.Regular, current.Regular) && slices.Equal(previous.Special, current.Special) && slices.Equal(previous.Digits, current.Digits)
}

func cloneDraw(draw rules.Draw) rules.Draw {
	draw.Regular = slices.Clone(draw.Regular)
	draw.Special = slices.Clone(draw.Special)
	draw.Digits = slices.Clone(draw.Digits)
	return draw
}
