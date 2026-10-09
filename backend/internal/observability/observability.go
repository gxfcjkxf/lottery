package observability

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultTraceRatio = 0.1
	workerFreshness   = 30 * time.Second
	listenerReadyTime = 2 * time.Second
)

// Options is the complete, explicit configuration for one service runtime.
type Options struct {
	Service       string
	Environment   string
	Version       string
	MetricsAddr   string
	TokenFile     string
	TraceEndpoint string
	TraceRatio    float64
}

// LoadEnv loads service-scoped settings. No OpenTelemetry SDK environment
// variables are read; all exporter configuration comes from these options.
func LoadEnv(service, environment, version string) (Options, error) {
	options := Options{
		Service:     service,
		Environment: environment,
		Version:     version,
		TraceRatio:  defaultTraceRatio,
	}
	metricsKey := "API_METRICS_ADDR"
	if service == "worker" {
		metricsKey = "WORKER_METRICS_ADDR"
	} else if service != "api" {
		return Options{}, errors.New("observability: service must be api or worker")
	}
	options.MetricsAddr = os.Getenv(metricsKey)
	options.TokenFile = os.Getenv("METRICS_TOKEN_FILE")
	options.TraceEndpoint = os.Getenv("TRACE_OTLP_ENDPOINT")
	if raw := os.Getenv("TRACE_SAMPLE_RATIO"); raw != "" {
		ratio, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
			return Options{}, errors.New("observability: TRACE_SAMPLE_RATIO must be a finite number from 0 through 1")
		}
		options.TraceRatio = ratio
	}
	if err := options.validate(); err != nil {
		return Options{}, err
	}
	return options, nil
}

func (o Options) validate() error {
	if o.Service != "api" && o.Service != "worker" {
		return errors.New("observability: service must be api or worker")
	}
	if o.Environment != "development" && o.Environment != "test" && o.Environment != "production" {
		return errors.New("observability: environment must be development, test, or production")
	}
	if len(o.Version) == 0 || len(o.Version) > 64 {
		return errors.New("observability: version must contain 1 through 64 safe characters")
	}
	for _, c := range o.Version {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c) {
			continue
		}
		return errors.New("observability: version must contain 1 through 64 safe characters")
	}
	if math.IsNaN(o.TraceRatio) || math.IsInf(o.TraceRatio, 0) || o.TraceRatio < 0 || o.TraceRatio > 1 {
		return errors.New("observability: trace ratio must be a finite number from 0 through 1")
	}
	if o.MetricsAddr != "" {
		if _, _, err := parseListenAddr(o.MetricsAddr); err != nil {
			return err
		}
		if o.TokenFile == "" {
			return errors.New("observability: METRICS_TOKEN_FILE is required when metrics are enabled")
		}
		if _, err := loadToken(o.TokenFile); err != nil {
			return err
		}
	}
	if o.TraceEndpoint != "" {
		if err := validateTraceEndpoint(o.TraceEndpoint); err != nil {
			return err
		}
	}
	return nil
}

func parseListenAddr(addr string) (string, string, error) {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", errors.New("observability: metrics address must be a literal IP and port")
	}
	ip := net.ParseIP(host)
	port, portErr := strconv.Atoi(portText)
	if ip == nil || portErr != nil || port < 1 || port > 65535 {
		return "", "", errors.New("observability: metrics address must use a literal IP and port 1 through 65535")
	}
	// This listener has no TLS. A bearer token does not protect transport on a
	// private LAN, so require a local collector or an explicitly secured proxy.
	if !ip.IsLoopback() {
		return "", "", errors.New("observability: plaintext metrics listeners require a loopback IP")
	}
	return ip.String(), strconv.Itoa(port), nil
}

func loadToken(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", errors.New("observability: cannot read metrics token file")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !validTokenMode(info.Mode()) {
		return "", errors.New("observability: metrics token file must be a regular non-symlink file with mode 0400 or 0600")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("observability: cannot read metrics token file")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if err != nil || pathErr != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || !os.SameFile(pathInfo, openedInfo) || !validTokenMode(openedInfo.Mode()) {
		return "", errors.New("observability: metrics token file must be a regular non-symlink file with mode 0400 or 0600")
	}
	data, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil {
		return "", errors.New("observability: cannot read metrics token file")
	}
	if len(data) < 32 || len(data) > 128 {
		return "", errors.New("observability: metrics token must contain 32 through 128 ASCII unreserved characters")
	}
	for _, b := range data {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-._~", rune(b))) {
			return "", errors.New("observability: metrics token must contain 32 through 128 ASCII unreserved characters")
		}
	}
	return string(data), nil
}

func validTokenMode(mode os.FileMode) bool {
	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	permissions := mode.Perm()
	return permissions == 0o400 || permissions == 0o600
}

func validateTraceEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "?#") || strings.HasSuffix(u.Host, ":") {
		return errors.New("observability: trace endpoint must be an HTTPS collector URL without credentials, query, or fragment")
	}
	if u.Scheme == "https" {
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return errors.New("observability: trace endpoint has an invalid port")
			}
		}
		return nil
	}
	if u.Scheme != "http" {
		return errors.New("observability: trace endpoint must use HTTPS (HTTP is allowed only for literal loopback tests)")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("observability: HTTP trace endpoints are allowed only on literal loopback IPs")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("observability: trace endpoint has an invalid port")
		}
	}
	return nil
}

type metrics struct {
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	workerRuns   *prometheus.CounterVec
	workerItems  *prometheus.CounterVec
	workerTime   *prometheus.HistogramVec
	workerLast   *prometheus.GaugeVec
	workerFlight *prometheus.GaugeVec
}

func newMetrics(options Options) *metrics {
	m := &metrics{registry: prometheus.NewRegistry()}
	m.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "lottery_http_requests_total", Help: "Completed HTTP requests."}, []string{"method", "route", "status"})
	m.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "lottery_http_request_duration_seconds", Help: "HTTP request duration in seconds."}, []string{"method", "route"})
	m.workerRuns = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "lottery_worker_runs_total", Help: "Completed worker runs."}, []string{"component", "outcome"})
	m.workerItems = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "lottery_worker_committed_items_total", Help: "Items committed by worker runs."}, []string{"component"})
	m.workerTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "lottery_worker_run_duration_seconds", Help: "Worker run duration in seconds.", Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}}, []string{"component"})
	m.workerLast = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lottery_worker_last_completed_timestamp_seconds", Help: "Unix timestamp of the last completed worker run."}, []string{"component"})
	m.workerFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lottery_worker_inflight", Help: "Worker runs currently in flight."}, []string{"component"})
	_ = m.registry.Register(m.httpRequests)
	_ = m.registry.Register(m.httpDuration)
	_ = m.registry.Register(m.workerRuns)
	_ = m.registry.Register(m.workerItems)
	_ = m.registry.Register(m.workerTime)
	_ = m.registry.Register(m.workerLast)
	_ = m.registry.Register(m.workerFlight)
	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lottery_build_info", Help: "Build identity."}, []string{"service", "environment", "version"})
	build.WithLabelValues(options.Service, options.Environment, options.Version).Set(1)
	_ = m.registry.Register(build)
	return m
}

// Runtime owns isolated Prometheus and OpenTelemetry providers for one service.
type Runtime struct {
	options  Options
	metrics  *metrics
	token    string
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	enabled  bool

	workerMu     sync.RWMutex
	completed    map[string]time.Time
	ready        atomic.Bool
	readyCheck   func(context.Context) error
	serverMu     sync.Mutex
	server       *http.Server
	listener     net.Listener
	stopOnce     sync.Once
	startCancel  context.CancelFunc
	shutdownOnce sync.Once
	shutdownErr  error
	errors       chan error
}

// New creates an isolated runtime. Options with MetricsAddr empty and no trace
// endpoint remain entirely disabled and do not alter global OpenTelemetry state.
func New(ctx context.Context, options Options) (*Runtime, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	r := &Runtime{options: options, completed: make(map[string]time.Time), errors: make(chan error, 1), enabled: options.MetricsAddr != "" || options.TraceEndpoint != ""}
	r.metrics = newMetrics(options)
	if options.MetricsAddr != "" {
		token, err := loadToken(options.TokenFile)
		if err != nil {
			return nil, err
		}
		r.token = token
	}
	if options.TraceEndpoint == "" {
		r.tracer = trace.NewNoopTracerProvider().Tracer("lottery")
		return r, nil
	}
	baseExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(options.TraceEndpoint),
		otlptracehttp.WithEncoding(otlptracehttp.EncodingProtobuf),
		otlptracehttp.WithHeaders(map[string]string{}),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{}),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression),
		otlptracehttp.WithTimeout(2*time.Second),
		otlptracehttp.WithHTTPClient(&http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}),
	)
	if err != nil {
		return nil, errors.New("observability: cannot initialize trace exporter")
	}
	exporter := &fixedResourceExporter{exporter: baseExporter, resource: resourceAttributes(options)}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(options.TraceRatio)),
		sdktrace.WithResource(resourceAttributes(options)),
		sdktrace.WithBatcher(exporter, sdktrace.WithMaxQueueSize(1024), sdktrace.WithMaxExportBatchSize(256), sdktrace.WithExportTimeout(2*time.Second), sdktrace.WithBatchTimeout(5*time.Second)),
	)
	r.provider = provider
	r.tracer = provider.Tracer("lottery")
	return r, nil
}

// resourceAttributes builds only fixed service identity attributes.
func resourceAttributes(o Options) *resource.Resource {
	return resource.NewWithAttributes("", attribute.String("service.name", o.Service), attribute.String("deployment.environment.name", o.Environment), attribute.String("service.version", o.Version))
}

// fixedResourceExporter removes SDK environment resource attributes at the
// external boundary, keeping only the explicitly configured service identity.
type fixedResourceExporter struct {
	exporter sdktrace.SpanExporter
	resource *resource.Resource
}

type fixedResourceSpan struct {
	sdktrace.ReadOnlySpan
	resource *resource.Resource
}

func (s fixedResourceSpan) Resource() *resource.Resource { return s.resource }

func (e *fixedResourceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	fixed := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		fixed[i] = fixedResourceSpan{ReadOnlySpan: span, resource: e.resource}
	}
	if err := e.exporter.ExportSpans(ctx, fixed); err != nil {
		// The SDK reports exporter failures through its global error handler.
		// Keep collector URLs, response bodies, and transport details out of logs.
		return errors.New("trace collector unavailable")
	}
	return nil
}

func (e *fixedResourceExporter) Shutdown(ctx context.Context) error { return e.exporter.Shutdown(ctx) }

// Enabled reports whether metrics listening or trace exporting is configured.
func (r *Runtime) Enabled() bool { return r != nil && r.enabled }

// Registry returns the runtime's private Prometheus registry.
func (r *Runtime) Registry() *prometheus.Registry {
	if r == nil || r.metrics == nil {
		return prometheus.NewRegistry()
	}
	return r.metrics.registry
}

type contextKey string

const (
	runtimeContextKey contextKey = "observability-runtime"
	routeContextKey   contextKey = "observability-route"
)

type routeState struct {
	mu      sync.RWMutex
	pattern string
}

// WithRuntime associates a runtime with a context for long-lived worker loops.
func WithRuntime(ctx context.Context, r *Runtime) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeContextKey, r)
}

// SetRoute records a static route pattern for the current request. It strips an
// optional HTTP method prefix and rejects values that cannot be canonical mux patterns.
func SetRoute(ctx context.Context, pattern string) {
	if ctx == nil {
		return
	}
	state, _ := ctx.Value(routeContextKey).(*routeState)
	if state == nil {
		return
	}
	if space := strings.IndexByte(pattern, ' '); space >= 0 {
		pattern = strings.TrimSpace(pattern[space+1:])
	}
	if !validRoutePattern(pattern) {
		return
	}
	state.mu.Lock()
	state.pattern = pattern
	state.mu.Unlock()
}

func validRoutePattern(pattern string) bool {
	if pattern == "" || pattern == "/" || len(pattern) > 200 || pattern[0] != '/' || strings.ContainsAny(pattern, "?#%") {
		return false
	}
	for _, c := range pattern {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/_-.{}*", c) {
			continue
		}
		return false
	}
	return true
}

func (s *routeState) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pattern == "" {
		return "unmatched"
	}
	return s.pattern
}

// TraceIDs returns IDs only from a valid OpenTelemetry span in ctx.
func TraceIDs(ctx context.Context) (traceID, spanID string) {
	if ctx == nil {
		return "", ""
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}

// BeginWork records one bounded worker component run using the runtime stored in ctx.
func BeginWork(ctx context.Context, component string) (context.Context, func(int, error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	r, _ := ctx.Value(runtimeContextKey).(*Runtime)
	return r.BeginWork(ctx, component)
}

func canonicalComponent(component string) string {
	switch component {
	case "calendar", "period_tick", "draw_collection", "period_refund", "settlement", "draw_correction", "notification", "reconciliation", "archive_discovery", "archive_processing", "commission_cycle", "commission_discovery", "commission_payment", "commission_correction_plan", "commission_correction_execution":
		return component
	default:
		return "other"
	}
}

// BeginWork starts a worker run and returns an idempotent completion function.
func (r *Runtime) BeginWork(ctx context.Context, component string) (context.Context, func(int, error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || !r.enabled || r.metrics == nil {
		return ctx, func(int, error) {}
	}
	component = canonicalComponent(component)
	started := time.Now()
	r.metrics.workerFlight.WithLabelValues(component).Inc()
	ctx, span := r.tracer.Start(ctx, "worker.run", trace.WithAttributes(attribute.String("worker.component", component)))
	var once sync.Once
	finish := func(committed int, runErr error) {
		once.Do(func() {
			if committed < 0 {
				committed = 0
			}
			outcome := "success"
			if runErr != nil {
				outcome = "error"
				span.SetStatus(codes.Error, "")
			}
			span.End()
			r.metrics.workerRuns.WithLabelValues(component, outcome).Inc()
			r.metrics.workerItems.WithLabelValues(component).Add(float64(committed))
			r.metrics.workerTime.WithLabelValues(component).Observe(time.Since(started).Seconds())
			completedAt := time.Now()
			r.metrics.workerLast.WithLabelValues(component).Set(float64(completedAt.UnixNano()) / 1e9)
			r.metrics.workerFlight.WithLabelValues(component).Dec()
			r.workerMu.Lock()
			r.completed[component] = completedAt
			r.workerMu.Unlock()
		})
	}
	return ctx, finish
}

// WorkerReady is true once both critical worker loops have completed within 30 seconds.
func (r *Runtime) WorkerReady(now time.Time) bool {
	if r == nil {
		return false
	}
	r.workerMu.RLock()
	defer r.workerMu.RUnlock()
	for _, component := range []string{"period_tick", "notification"} {
		completedAt, ok := r.completed[component]
		if !ok || now.Before(completedAt) || now.Sub(completedAt) > workerFreshness {
			return false
		}
	}
	return true
}

// WrapHTTP records bounded route/method/status and duration dimensions.
func (r *Runtime) WrapHTTP(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	if r == nil || !r.enabled || r.metrics == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		method := canonicalMethod(req.Method)
		started := time.Now()
		route := &routeState{}
		baseCtx := context.WithValue(req.Context(), routeContextKey, route)
		baseCtx = propagation.TraceContext{}.Extract(baseCtx, propagation.HeaderCarrier(req.Header))
		// Preserve validated parent IDs and flags, not arbitrary vendor strings
		// supplied in tracestate. Baggage is never extracted.
		parent := trace.SpanContextFromContext(baseCtx)
		if parent.IsValid() {
			baseCtx = trace.ContextWithSpanContext(baseCtx, parent.WithTraceState(trace.TraceState{}))
		}
		ctx, span := r.tracer.Start(baseCtx, "http.request", trace.WithAttributes(attribute.String("http.request.method", method)))
		tracked := &statusWriter{ResponseWriter: w}
		defer func() {
			recovered := recover()
			if recovered != nil {
				if !tracked.wroteFinal {
					tracked.WriteHeader(http.StatusInternalServerError)
				} else {
					tracked.status = http.StatusInternalServerError
				}
			}
			pattern := route.get()
			status := tracked.status
			if status == 0 {
				status = http.StatusOK
			}
			r.metrics.httpRequests.WithLabelValues(method, pattern, strconv.Itoa(status)).Inc()
			r.metrics.httpDuration.WithLabelValues(method, pattern).Observe(time.Since(started).Seconds())
			span.SetName(method + " " + pattern)
			span.SetAttributes(attribute.String("http.route", pattern), attribute.Int("http.response.status_code", status))
			if status >= 500 {
				span.SetStatus(codes.Error, "")
			}
			span.End()
			if recovered != nil {
				panic(recovered)
			}
		}()
		next.ServeHTTP(tracked, req.WithContext(ctx))
	})
}

func canonicalMethod(method string) string {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return method
	default:
		return "other"
	}
}

// MethodLabel returns the bounded HTTP method value used in telemetry and logs.
func MethodLabel(method string) string { return canonicalMethod(method) }

type statusWriter struct {
	http.ResponseWriter
	status     int
	wroteFinal bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteFinal {
		return
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.ResponseWriter.WriteHeader(code)
	w.status, w.wroteFinal = code, true
}

func (w *statusWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if !w.wroteFinal {
		w.status, w.wroteFinal = http.StatusOK, true
	}
	return n, err
}

func (w *statusWriter) Flush() {
	_ = w.FlushError()
}

func (w *statusWriter) FlushError() error {
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		return err
	}
	if !w.wroteFinal {
		w.status, w.wroteFinal = http.StatusOK, true
	}
	return nil
}

func (w *statusWriter) ReadFrom(reader io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{w}, reader)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("http: hijacker not supported")
	}
	return hijacker.Hijack()
}

func (w *statusWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

// MetricsHandler serves only authenticated metrics and health endpoints.
func (r *Runtime) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r == nil {
			writeJSON(w, http.StatusNotFound, `{"status":"not_found"}`)
			return
		}
		provided := req.Header.Get("Authorization")
		wantHash := sha256.Sum256([]byte("Bearer " + r.token))
		gotHash := sha256.Sum256([]byte(provided))
		if r.token == "" || subtle.ConstantTimeCompare(wantHash[:], gotHash[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, `{"status":"unauthorized"}`)
			return
		}
		if r.options.MetricsAddr == "" {
			writeJSON(w, http.StatusNotFound, `{"status":"not_found"}`)
			return
		}
		switch req.URL.Path {
		case "/metrics":
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				methodNotAllowed(w)
				return
			}
			promhttp.HandlerFor(r.metrics.registry, promhttp.HandlerOpts{DisableCompression: true}).ServeHTTP(w, req)
		case "/health/live":
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				methodNotAllowed(w)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if req.Method != http.MethodHead {
				_, _ = io.WriteString(w, "{\"status\":\"live\"}\n")
			}
		case "/health/ready":
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				methodNotAllowed(w)
				return
			}
			ready := r.ready.Load()
			if ready && r.readyCheck != nil {
				checkCtx, cancel := context.WithTimeout(req.Context(), listenerReadyTime)
				checkErr := r.readyCheck(checkCtx)
				ready = checkErr == nil && checkCtx.Err() == nil
				cancel()
			}
			if r.options.Service == "worker" {
				ready = ready && r.WorkerReady(time.Now())
			}
			if !ready {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				if req.Method != http.MethodHead {
					_, _ = io.WriteString(w, "{\"status\":\"not_ready\"}\n")
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if req.Method != http.MethodHead {
				_, _ = io.WriteString(w, "{\"status\":\"ready\"}\n")
			}
		default:
			writeJSON(w, http.StatusNotFound, `{"status":"not_found"}`)
		}
	})
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, HEAD")
	writeJSON(w, http.StatusMethodNotAllowed, `{"status":"method_not_allowed"}`)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != "" {
		_, _ = io.WriteString(w, body+"\n")
	}
}

// Start opens the metrics listener synchronously, then serves it in the background.
// Readiness has a hard two-second deadline. Worker readiness also requires fresh core-loop completions.
func (r *Runtime) Start(ctx context.Context, ready func(context.Context) error) (func(context.Context) error, error) {
	if r == nil {
		return func(context.Context) error { return nil }, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.options.MetricsAddr == "" {
		return func(context.Context) error { return nil }, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.New("observability: startup context canceled")
	}
	if r.options.MetricsAddr != "" {
		host, port, _ := parseListenAddr(r.options.MetricsAddr)
		listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err != nil {
			return nil, fmt.Errorf("observability: metrics listener failed: %w", err)
		}
		r.listener = listener
		r.server = &http.Server{Handler: r.MetricsHandler(), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 1 << 14}
		go func() {
			if err := r.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				select {
				case r.errors <- errors.New("observability: metrics listener stopped unexpectedly"):
				default:
				}
			}
		}()
	}
	serviceCtx, serviceCancel := context.WithCancel(ctx)
	r.startCancel = serviceCancel
	r.readyCheck = ready
	deadlineCtx, cancel := context.WithTimeout(serviceCtx, listenerReadyTime)
	defer cancel()
	if ready != nil {
		if err := ready(deadlineCtx); err != nil {
			_ = r.stopServer(context.Background())
			return nil, fmt.Errorf("observability: readiness failed: %w", err)
		}
	}
	if err := deadlineCtx.Err(); err != nil {
		_ = r.stopServer(context.Background())
		return nil, errors.New("observability: readiness deadline exceeded")
	}
	r.ready.Store(true)
	stop := func(stopCtx context.Context) error { return r.stopServer(stopCtx) }
	go func() { <-serviceCtx.Done(); _ = r.stopServer(context.Background()) }()
	return stop, nil
}

func (r *Runtime) stopServer(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var err error
	r.stopOnce.Do(func() {
		r.ready.Store(false)
		if r.startCancel != nil {
			r.startCancel()
		}
		r.serverMu.Lock()
		server := r.server
		listener := r.listener
		r.serverMu.Unlock()
		if server != nil {
			err = server.Shutdown(bounded)
			if err != nil {
				_ = server.Close()
			}
		} else if listener != nil {
			err = listener.Close()
		}
	})
	return err
}

// Errors reports unexpected background listener failures. The channel remains open.
func (r *Runtime) Errors() <-chan error {
	if r == nil {
		return nil
	}
	return r.errors
}

// Shutdown flushes and closes the private trace provider with a five-second bound.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || r.provider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.shutdownOnce.Do(func() { r.shutdownErr = r.provider.Shutdown(bounded) })
	if r.shutdownErr != nil {
		return errors.New("observability: trace shutdown failed")
	}
	return nil
}
