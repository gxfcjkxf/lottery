package observability

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

const testToken = "0123456789abcdefghijklmnopqrstuvwxyzABCDEF"

func tokenFile(t *testing.T, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.token")
	if err := os.WriteFile(path, []byte(testToken), mode); err != nil {
		t.Fatal("could not prepare private test token")
	}
	return path
}

func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{Service: "api", Environment: "test", Version: "test-build", MetricsAddr: "127.0.0.1:19442", TokenFile: tokenFile(t, 0o600), TraceRatio: 1}
}

func TestLoadEnvDefaultsAndValidation(t *testing.T) {
	for _, name := range []string{"API_METRICS_ADDR", "WORKER_METRICS_ADDR", "METRICS_TOKEN_FILE", "TRACE_OTLP_ENDPOINT", "TRACE_SAMPLE_RATIO", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS"} {
		t.Setenv(name, "")
	}
	options, err := LoadEnv("api", "production", "sha")
	if err != nil {
		t.Fatal("default environment configuration rejected")
	}
	if options.MetricsAddr != "" || options.TraceEndpoint != "" || options.TraceRatio != 0.1 {
		t.Fatalf("unexpected default configuration: %+v", options)
	}
	if _, err = LoadEnv("worker", "production", "sha"); err != nil {
		t.Fatal("worker defaults rejected")
	}
	if _, err = LoadEnv("scheduler", "production", "sha"); err == nil {
		t.Fatal("unknown service accepted")
	}
	if _, err = LoadEnv("api", "staging", "sha"); err == nil {
		t.Fatal("unknown environment accepted")
	}
	for _, ratio := range []string{"NaN", "+Inf", "-Inf", "-0.01", "1.01", "bad"} {
		t.Setenv("TRACE_SAMPLE_RATIO", ratio)
		if _, err = LoadEnv("api", "test", "v"); err == nil {
			t.Fatalf("invalid ratio %q accepted", ratio)
		}
	}
	t.Setenv("TRACE_SAMPLE_RATIO", "1")
	if options, err = LoadEnv("api", "test", "v"); err != nil || options.TraceRatio != 1 {
		t.Fatal("ratio boundary 1 rejected")
	}
	t.Setenv("TRACE_SAMPLE_RATIO", "0")
	if options, err = LoadEnv("api", "test", "v"); err != nil || options.TraceRatio != 0 {
		t.Fatal("ratio boundary 0 rejected")
	}
}

func TestTokenFileValidationAndAddressParsing(t *testing.T) {
	good := tokenFile(t, 0o600)
	if _, err := loadToken(good); err != nil {
		t.Fatal("private token file rejected")
	}
	readOnly := tokenFile(t, 0o400)
	if _, err := loadToken(readOnly); err != nil {
		t.Fatal("owner-read-only token file rejected")
	}
	badMode := tokenFile(t, 0o640)
	if _, err := loadToken(badMode); err == nil {
		t.Fatal("group-readable token accepted")
	}
	executable := tokenFile(t, 0o700)
	if _, err := loadToken(executable); err == nil {
		t.Fatal("owner-executable token accepted")
	}
	if validTokenMode(os.FileMode(0o600) | os.ModeSetuid) {
		t.Fatal("setuid token mode accepted")
	}
	short := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(short, []byte("short"), 0o600); err != nil {
		t.Fatal("could not prepare token fixture")
	}
	if _, err := loadToken(short); err == nil {
		t.Fatal("short token accepted")
	}
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(good, symlink); err != nil {
		t.Fatal("could not prepare symlink fixture")
	}
	if _, err := loadToken(symlink); err == nil {
		t.Fatal("symlink token accepted")
	}
	for _, addr := range []string{"localhost:1234", "example.com:1234", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1", "[::1]:bad", "0.0.0.0:1234", "[::]:1234", "192.168.1.2:1234", "10.0.0.2:1234", "192.0.2.4:1234", "[2001:db8::2]:1234"} {
		if _, _, err := parseListenAddr(addr); err == nil {
			t.Fatalf("invalid listen address accepted: %s", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:1234", "127.0.0.2:1234", "[::1]:1234", "[::ffff:127.0.0.1]:1234"} {
		if _, _, err := parseListenAddr(addr); err != nil {
			t.Fatalf("literal IP address rejected: %s", addr)
		}
	}
	for _, endpoint := range []string{"https://collector.example/v1/traces", "http://127.0.0.1:4318/v1/traces", "http://[::1]:4318/v1/traces"} {
		if err := validateTraceEndpoint(endpoint); err != nil {
			t.Fatalf("valid trace endpoint rejected: %s", endpoint)
		}
	}
	for _, endpoint := range []string{"http://collector.example/v1/traces", "http://localhost:4318/v1/traces", "https://user:pass@collector.example/v1/traces", "https://collector.example/v1/traces?token=x", "https://collector.example/v1/traces#frag", "ftp://collector.example/traces"} {
		if err := validateTraceEndpoint(endpoint); err == nil {
			t.Fatalf("invalid trace endpoint accepted: %s", endpoint)
		}
	}
}

func TestHTTPRouteHandoffAndCardinality(t *testing.T) {
	runtime, err := New(context.Background(), testOptions(t))
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	defer runtime.Shutdown(context.Background())
	handler := runtime.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Simulate inner middleware replacing the request while retaining its context.
		cloned := req.WithContext(context.WithValue(req.Context(), struct{}{}, "middleware"))
		SetRoute(cloned.Context(), "GET /users/{userID}")
		w.WriteHeader(http.StatusCreated)
	}))
	request := httptest.NewRequest(http.MethodPost, "/users/private-param?secret=query-value", nil)
	request.Pattern = "/spoof/{raw}"
	request.Header.Set("X-Request-ID", "private-header")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if got := counterValue(t, runtime.metrics.httpRequests.WithLabelValues("POST", "/users/{userID}", "201")); got != 1 {
		t.Fatalf("canonical route handoff not recorded: %v", got)
	}
	for i := 0; i < 1200; i++ {
		request = httptest.NewRequest("CUSTOM-"+strconv.Itoa(i), "/random/"+strconv.Itoa(i)+"?private=value", nil)
		runtime.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })).ServeHTTP(httptest.NewRecorder(), request)
	}
	if got := counterValue(t, runtime.metrics.httpRequests.WithLabelValues("other", "unmatched", "404")); got != 1200 {
		t.Fatalf("unknown route/method count mismatch: %v", got)
	}
	metricCount := 0
	seenCanonical, seenUnmatched, seenSpoof := false, false, false
	for _, family := range mustGather(t, runtime) {
		if family.GetName() == "lottery_http_requests_total" {
			metricCount = len(family.Metric)
			for _, metric := range family.Metric {
				labels := map[string]string{}
				for _, label := range metric.Label {
					labels[label.GetName()] = label.GetValue()
				}
				switch labels["route"] {
				case "/users/{userID}":
					seenCanonical = true
				case "unmatched":
					seenUnmatched = true
				case "/spoof/{raw}":
					seenSpoof = true
				}
			}
		}
	}
	if metricCount != 2 || !seenCanonical || !seenUnmatched || seenSpoof {
		t.Fatalf("request label cardinality = %d, want 2", metricCount)
	}
}

func mustGather(t *testing.T, runtime *Runtime) []*dto.MetricFamily {
	t.Helper()
	families, err := runtime.Registry().Gather()
	if err != nil {
		t.Fatal("private registry gather failed")
	}
	return families
}

func counterValue(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		t.Fatal("counter read failed")
	}
	return metric.GetCounter().GetValue()
}

func TestHTTPPanicAndWriterSemantics(t *testing.T) {
	runtime, err := New(context.Background(), testOptions(t))
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	defer runtime.Shutdown(context.Background())
	func() {
		defer func() {
			if recover() != "boom" {
				t.Fatal("handler panic was not rethrown")
			}
		}()
		runtime.WrapHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/private/path", nil))
	}()
	if got := counterValue(t, runtime.metrics.httpRequests.WithLabelValues("GET", "unmatched", "500")); got != 1 {
		t.Fatalf("panic did not record 500: %v", got)
	}
	invalidServer := httptest.NewServer(runtime.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer func() {
			if recover() != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		w.WriteHeader(1000)
	})))
	response, err := invalidServer.Client().Get(invalidServer.URL + "/invalid")
	if err != nil {
		invalidServer.Close()
		t.Fatal("invalid status recovery closed the response")
	}
	_ = response.Body.Close()
	invalidServer.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("invalid status recovery returned %d", response.StatusCode)
	}
	if got := counterValue(t, runtime.metrics.httpRequests.WithLabelValues("GET", "unmatched", "500")); got != 2 {
		t.Fatalf("invalid status was not recovered as 500: %v", got)
	}

	underlying := &semanticWriter{}
	w := &statusWriter{ResponseWriter: underlying}
	w.WriteHeader(103)
	w.WriteHeader(201)
	w.WriteHeader(500)
	if w.status != 201 || underlying.final != 201 {
		t.Fatalf("informational status handling failed: wrapper=%d writer=%d", w.status, underlying.final)
	}
	if unwrapper, ok := interface{}(w).(interface{ Unwrap() http.ResponseWriter }); !ok || unwrapper.Unwrap() != underlying {
		t.Fatal("Unwrap was not preserved")
	}
	w.Flush()
	if !underlying.flushed {
		t.Fatal("Flush was not forwarded")
	}
	if _, err := w.ReadFrom(bytes.NewBufferString("reader-from")); err != nil {
		t.Fatal("ReadFrom failed")
	}
	if underlying.writeBytes != len("reader-from") {
		t.Fatal("ReadFrom did not write its content")
	}
	emptyUnderlying := &semanticWriter{}
	emptyWriter := &statusWriter{ResponseWriter: emptyUnderlying}
	if n, err := emptyWriter.ReadFrom(strings.NewReader("")); n != 0 || err != nil {
		t.Fatal("empty ReadFrom failed")
	}
	emptyWriter.WriteHeader(201)
	if emptyWriter.status != 201 || emptyUnderlying.final != 201 {
		t.Fatal("empty ReadFrom committed an implicit status")
	}
	unsupported := &statusWriter{ResponseWriter: &bareWriter{header: make(http.Header)}}
	if err := unsupported.FlushError(); !errors.Is(err, http.ErrNotSupported) {
		t.Fatal("unsupported flush did not return ErrNotSupported")
	}
	if unsupported.wroteFinal {
		t.Fatal("unsupported flush committed an implicit status")
	}
}

type semanticWriter struct {
	header     http.Header
	final      int
	flushed    bool
	readFrom   int64
	writeBytes int
}

type bareWriter struct {
	header http.Header
	status int
}

func (w *bareWriter) Header() http.Header         { return w.header }
func (w *bareWriter) WriteHeader(status int)      { w.status = status }
func (w *bareWriter) Write(p []byte) (int, error) { return len(p), nil }

func (w *semanticWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *semanticWriter) WriteHeader(code int) {
	if code >= 200 || code == 101 {
		w.final = code
	}
}
func (w *semanticWriter) Write(p []byte) (int, error) {
	if w.final == 0 {
		w.final = 200
	}
	w.writeBytes += len(p)
	return len(p), nil
}
func (w *semanticWriter) Flush() { w.flushed = true }
func (w *semanticWriter) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(io.Discard, r)
	w.readFrom += n
	return n, err
}

func TestMetricsHandlerAuthenticationAndPrivateHealth(t *testing.T) {
	runtime, err := New(context.Background(), testOptions(t))
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	defer runtime.Shutdown(context.Background())
	handler := runtime.MetricsHandler()
	for _, path := range []string{"/metrics?token=" + testToken, "/health/live", "/health/ready", "/not-a-route"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), testToken) {
			t.Fatalf("unauthenticated endpoint response unsafe: %d", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("unauthenticated response missed security headers")
		}
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/health/live", 200}, {"/health/ready", 503}, {"/metrics", 200}, {"/private", 404}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", tc.path, nil)
		if tc.path == "/metrics" {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		handler.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s returned %d, want %d", tc.path, w.Code, tc.status)
		}
		if strings.Contains(w.Body.String(), "private") && tc.path != "/metrics" {
			t.Fatalf("health response leaked private data: %q", w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("authenticated response missed security headers")
		}
		if tc.path == "/metrics" && w.Header().Get("Content-Encoding") != "" {
			t.Fatal("metrics response unexpectedly compressed")
		}
	}
	wrong := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/live", nil)
	req.Header.Set("Authorization", "Bearer "+testToken+"x")
	handler.ServeHTTP(wrong, req)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatal("non-exact bearer token accepted")
	}
	runtime.ready.Store(true)
	runtime.readyCheck = func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness callback was not bounded")
		}
		return errors.New("private database error")
	}
	failedReady := httptest.NewRecorder()
	readyReq := httptest.NewRequest("GET", "/health/ready", nil)
	readyReq.Header.Set("Authorization", "Bearer "+testToken)
	handler.ServeHTTP(failedReady, readyReq)
	if failedReady.Code != http.StatusServiceUnavailable || strings.Contains(failedReady.Body.String(), "private database error") {
		t.Fatal("readiness failure was not private")
	}
	runtime.readyCheck = func(context.Context) error { return nil }
	passedReady := httptest.NewRecorder()
	readyReq = httptest.NewRequest("GET", "/health/ready", nil)
	readyReq.Header.Set("Authorization", "Bearer "+testToken)
	handler.ServeHTTP(passedReady, readyReq)
	if passedReady.Code != http.StatusOK {
		t.Fatal("fresh successful readiness callback was not observed")
	}
}

func TestWorkerMetricsReadinessAndContextNoop(t *testing.T) {
	disabled, err := New(context.Background(), Options{Service: "worker", Environment: "test", Version: "v", TraceRatio: 1})
	if err != nil {
		t.Fatal("disabled runtime initialization failed")
	}
	if disabled.Enabled() {
		t.Fatal("default telemetry should be disabled")
	}
	unauthenticated := httptest.NewRecorder()
	disabled.MetricsHandler().ServeHTTP(unauthenticated, httptest.NewRequest("GET", "/health/live", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatal("disabled metrics handler exposed an unauthenticated route")
	}
	stop, err := disabled.Start(context.Background(), nil)
	if err != nil || stop == nil {
		t.Fatal("disabled Start did not return a no-op stop function")
	}
	if err := stop(context.Background()); err != nil {
		t.Fatal("disabled stop function failed")
	}
	_, finish := disabled.BeginWork(context.Background(), "period_tick")
	finish(9, nil)
	for _, family := range mustGather(t, disabled) {
		if family.GetName() == "lottery_worker_runs_total" && len(family.Metric) != 0 {
			t.Fatal("disabled worker runtime recorded metrics")
		}
	}
	options := testOptions(t)
	options.Service = "worker"
	runtime, err := New(context.Background(), options)
	if err != nil {
		t.Fatal("enabled worker runtime initialization failed")
	}
	defer runtime.Shutdown(context.Background())
	now := time.Now()
	if runtime.WorkerReady(now) {
		t.Fatal("cold worker reported ready")
	}
	ctx := WithRuntime(context.Background(), runtime)
	ctx, finish = BeginWork(ctx, "period_tick")
	finish(7, context.Canceled)
	_, finish = BeginWork(ctx, "notification")
	finish(3, nil)
	if !runtime.WorkerReady(time.Now()) {
		t.Fatal("completed core loops did not report ready")
	}
	if runtime.WorkerReady(time.Now().Add(-time.Second)) {
		t.Fatal("future-dated loop completions reported ready")
	}
	if runtime.WorkerReady(now.Add(31 * time.Second)) {
		t.Fatal("stale worker loops reported ready")
	}
	if got := counterValue(t, runtime.metrics.workerRuns.WithLabelValues("period_tick", "error")); got != 1 {
		t.Fatalf("run error outcome not recorded: %v", got)
	}
	if got := counterValue(t, runtime.metrics.workerItems.WithLabelValues("period_tick")); got != 7 {
		t.Fatalf("committed items on failed run lost: %v", got)
	}
	last := &dto.Metric{}
	if err := runtime.metrics.workerLast.WithLabelValues("period_tick").Write(last); err != nil || last.GetGauge().GetValue() <= 0 {
		t.Fatal("worker completion timestamp was not recorded")
	}
	duration := &dto.Metric{}
	if err := runtime.metrics.workerTime.WithLabelValues("calendar").(prometheus.Histogram).Write(duration); err != nil {
		t.Fatal("worker duration histogram unavailable")
	}
	has30, has60 := false, false
	for _, bucket := range duration.GetHistogram().Bucket {
		has30 = has30 || bucket.GetUpperBound() == 30
		has60 = has60 || bucket.GetUpperBound() == 60
	}
	if !has30 || !has60 {
		t.Fatal("worker duration histogram is missing the 30s and 60s buckets")
	}
	if got := counterValue(t, runtime.metrics.workerRuns.WithLabelValues("other", "success")); got != 0 {
		t.Fatalf("unstarted unknown component label unexpectedly materialized: %v", got)
	}
	_, finish = runtime.BeginWork(context.Background(), "unknown-private-name")
	finish(-9, nil)
	if got := counterValue(t, runtime.metrics.workerItems.WithLabelValues("other")); got != 0 {
		t.Fatalf("negative committed count was not clamped: %v", got)
	}
	if traceID, spanID := TraceIDs(context.Background()); traceID != "" || spanID != "" {
		t.Fatal("invalid span context returned IDs")
	}
	if MethodLabel("CUSTOM-EXTENSION") != "other" || MethodLabel("GET") != "GET" {
		t.Fatal("HTTP method label is not bounded")
	}
	if _, finish := BeginWork(context.Background(), "other"); finish == nil {
		t.Fatal("nil context runtime did not return no-op")
	}
}

func TestStartListenerFailureAndReadiness(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("could not reserve listener")
	}
	addr := occupied.Addr().String()
	options := testOptions(t)
	options.MetricsAddr = addr
	runtime, err := New(context.Background(), options)
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	if _, err = runtime.Start(context.Background(), nil); err == nil {
		t.Fatal("occupied listener did not fail startup")
	}
	_ = occupied.Close()

	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("could not reserve ephemeral port")
	}
	options.MetricsAddr = reserve.Addr().String()
	_ = reserve.Close()
	runtime, err = New(context.Background(), options)
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	stop, err := runtime.Start(context.Background(), func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal("listener did not start with successful readiness")
	}
	if err = stop(context.Background()); err != nil {
		t.Fatal("listener did not stop cleanly")
	}
	if err = runtime.Shutdown(context.Background()); err != nil {
		t.Fatal("runtime shutdown failed")
	}
}

func TestOTLPHTTPExportUsesOnlyExplicitEndpointAndNoCredentials(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1/ambient")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:1/ambient-traces")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=ambient-secret")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "Authorization=ambient-traces-secret")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "credential=ambient-resource")
	t.Setenv("OTEL_SERVICE_NAME", "ambient-service")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	var mu sync.Mutex
	var payload []byte
	var auth, traceHeaders string
	var hits int
	var httpTraceID string
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hits++
		auth = req.Header.Get("Authorization")
		traceHeaders = req.Header.Get("X-Request-ID")
		payload, _ = io.ReadAll(req.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	options := Options{Service: "worker", Environment: "test", Version: "build-1", TraceEndpoint: collector.URL + "/v1/traces", TraceRatio: 1}
	runtime, err := New(context.Background(), options)
	if err != nil {
		t.Fatal("explicit loopback trace exporter did not initialize")
	}
	ctx := WithRuntime(context.Background(), runtime)
	ctx, finish := BeginWork(ctx, "notification")
	finish(2, nil)
	handler := runtime.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if id, sid := TraceIDs(req.Context()); len(id) != 32 || len(sid) != 16 {
			t.Error("HTTP request did not expose valid SDK trace IDs")
		} else {
			httpTraceID = id
		}
		SetRoute(req.Context(), "GET /fixed/{id}")
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest("GET", "/fixed/private-value?secret=query-secret", nil)
	request.Header.Set("X-Request-ID", "private-header-value")
	request.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	request.Header.Set("Baggage", "tenant=private-baggage")
	request.Header.Set("Tracestate", "vendor=private-vendor-state")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if err = runtime.Shutdown(context.Background()); err != nil {
		t.Fatal("OTLP shutdown failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("trace exporter sent %d batches, want one", hits)
	}
	if auth != "" || traceHeaders != "" {
		t.Fatalf("unexpected exporter headers: authorizationPresent=%t requestIDPresent=%t", auth != "", traceHeaders != "")
	}
	var exported collectortrace.ExportTraceServiceRequest
	if err := proto.Unmarshal(payload, &exported); err != nil {
		t.Fatal("collector did not receive protobuf OTLP")
	}
	if len(exported.ResourceSpans) != 1 || len(exported.ResourceSpans[0].ScopeSpans) != 1 || len(exported.ResourceSpans[0].ScopeSpans[0].Spans) != 2 {
		t.Fatal("expected one HTTP and one worker span")
	}
	resourceAttrs := exported.ResourceSpans[0].Resource.Attributes
	if len(resourceAttrs) != 3 {
		t.Fatalf("exported resource has %d attributes, want only three fixed identity fields", len(resourceAttrs))
	}
	resourceValues := map[string]string{}
	for _, attr := range resourceAttrs {
		resourceValues[attr.Key] = attr.Value.GetStringValue()
	}
	if resourceValues["service.name"] != "worker" || resourceValues["deployment.environment.name"] != "test" || resourceValues["service.version"] != "build-1" {
		t.Fatal("exported resource did not contain the explicit service identity")
	}
	traceIDBytes := []byte{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	parentSpanBytes := []byte{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
	propagated := false
	for _, span := range exported.ResourceSpans[0].ScopeSpans[0].Spans {
		if span.Name != "GET /fixed/{id}" {
			continue
		}
		if !bytes.Equal(span.TraceId, traceIDBytes) || !bytes.Equal(span.ParentSpanId, parentSpanBytes) || httpTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatal("valid W3C trace parent was not propagated")
		}
		propagated = true
	}
	if !propagated {
		t.Fatal("canonical HTTP span was not exported")
	}
	encoded := string(payload)
	for _, private := range []string{"private-value", "query-secret", "private-header-value", "private-baggage", "private-vendor-state", "ambient-secret", "ambient-traces-secret", "ambient-resource", "ambient-service"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("exported telemetry contains private input %q", private)
		}
	}
}

func TestTraceExporterDoesNotRetry(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer collector.Close()
	runtime, err := New(context.Background(), Options{Service: "api", Environment: "test", Version: "build", TraceEndpoint: collector.URL + "/v1/traces", TraceRatio: 1})
	if err != nil {
		t.Fatal("loopback exporter initialization failed")
	}
	_, finish := runtime.BeginWork(context.Background(), "calendar")
	finish(0, nil)
	_ = runtime.Shutdown(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("exporter retried failed batch %d times", hits-1)
	}
}

func TestConcurrentWorkerAndHTTPRecording(t *testing.T) {
	options := testOptions(t)
	options.Service = "worker"
	runtime, err := New(context.Background(), options)
	if err != nil {
		t.Fatal("runtime initialization failed")
	}
	defer runtime.Shutdown(context.Background())
	handler := runtime.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		SetRoute(req.Context(), "GET /concurrent/{id}")
		w.WriteHeader(http.StatusNoContent)
	}))
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, finish := runtime.BeginWork(context.Background(), "period_tick")
			_, notifyFinish := runtime.BeginWork(ctx, "notification")
			finish(1, nil)
			notifyFinish(1, nil)
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/concurrent/private", nil))
		}()
	}
	group.Wait()
	if !runtime.WorkerReady(time.Now()) {
		t.Fatal("concurrent core-loop completions did not report ready")
	}
	if got := counterValue(t, runtime.metrics.workerRuns.WithLabelValues("period_tick", "success")); got != 32 {
		t.Fatalf("concurrent worker completions lost: %v", got)
	}
}
