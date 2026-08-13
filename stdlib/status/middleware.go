// Package status provides request tracing and HTTP server status pages.
package status

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// ServerStatusPath is the default status overview served by ServerStatus.
const ServerStatusPath = "/debug/server-status"

const (
	ServerStatusLivePath   = ServerStatusPath + "/live"
	ServerStatusLogPath    = ServerStatusPath + "/log"
	ServerStatusStatsPath  = ServerStatusPath + "/stats"
	ServerStatusDetailPath = ServerStatusPath + "/detail/"
)

const (
	historyLimit = 100
	topLimit     = 20
)

type requestIDKey struct{}

// RequestID returns the ULID assigned to the request in ctx.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Request describes an active or recently completed request.
type Request struct {
	ID             string        `json:"request_id"`
	Status         runner.Status `json:"status"`
	Request        string        `json:"request"`
	Hostname       string        `json:"hostname"`
	Filename       string        `json:"filename,omitempty"`
	IncludedFiles  int           `json:"included_files"`
	Method         string        `json:"method"`
	URI            string        `json:"uri"`
	Protocol       string        `json:"protocol"`
	RemoteAddress  string        `json:"remote_address"`
	UserAgent      string        `json:"user_agent,omitempty"`
	StartedAt      time.Time     `json:"started_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Duration       time.Duration `json:"duration_ns"`
	ResponseStatus int           `json:"response_status,omitempty"`
	ResponseBytes  int64         `json:"response_bytes"`
	HeapDelta      int64         `json:"heap_delta_bytes"`
	AllocatedBytes uint64        `json:"allocated_bytes"`
	Allocations    uint64        `json:"allocations"`
	GCCycles       uint32        `json:"gc_cycles"`
	GCPause        time.Duration `json:"gc_pause_ns"`
	Spans          []RequestSpan `json:"spans,omitempty"`

	mem            runtime.MemStats
	stateChangedAt time.Time
}

// MemorySnapshot describes current process memory and GC pressure.
type MemorySnapshot struct {
	HeapAlloc     uint64  `json:"heap_alloc_bytes"`
	HeapInuse     uint64  `json:"heap_inuse_bytes"`
	HeapObjects   uint64  `json:"heap_objects"`
	StackInuse    uint64  `json:"stack_inuse_bytes"`
	System        uint64  `json:"system_bytes"`
	NextGC        uint64  `json:"next_gc_bytes"`
	NumGC         uint32  `json:"gc_cycles"`
	GCPauseTotal  uint64  `json:"gc_pause_total_ns"`
	GCCPUFraction float64 `json:"gc_cpu_fraction"`
	MemoryLimit   uint64  `json:"memory_limit_bytes,omitempty"`
}

// PoolEstimate is a heuristic concurrency estimate based on observed request
// allocations. Allocations are process-wide, so concurrent requests overlap.
type PoolEstimate struct {
	Samples               uint64 `json:"samples"`
	AverageAllocatedBytes uint64 `json:"average_allocated_bytes"`
	BeforeNextGC          uint64 `json:"requests_before_next_gc,omitempty"`
	WithinMemoryLimit     uint64 `json:"requests_within_memory_limit,omitempty"`
}

// RequestStatistic aggregates one method and URI in the rolling window.
type RequestStatistic struct {
	Request               string        `json:"request"`
	Hostname              string        `json:"hostname"`
	Filename              string        `json:"filename,omitempty"`
	AverageIncludedFiles  float64       `json:"average_included_files"`
	Count                 uint64        `json:"count"`
	Share                 float64       `json:"share_percent"`
	AverageDuration       time.Duration `json:"average_duration_ns"`
	AverageResponseBytes  uint64        `json:"average_response_bytes"`
	AverageAllocatedBytes uint64        `json:"average_allocated_bytes"`

	totalDuration      time.Duration
	totalResponseBytes uint64
	totalAllocated     uint64
	totalIncluded      uint64
}

// StatisticsSnapshot contains the most frequent requests in the rolling
// completed-request window.
type StatisticsSnapshot struct {
	WindowSize  int                `json:"window_size"`
	WindowLimit int                `json:"window_limit"`
	TopLimit    int                `json:"top_limit"`
	Top         []RequestStatistic `json:"top"`
}

// StateDuration is the lifetime request time observed in one scoreboard state.
type StateDuration struct {
	State    runner.Status `json:"state"`
	Label    string        `json:"label"`
	Duration time.Duration `json:"duration_ns"`
	Share    float64       `json:"share_percent"`
}

// Snapshot is the complete server-status document.
type Snapshot struct {
	StartedAt   time.Time          `json:"started_at"`
	Uptime      time.Duration      `json:"uptime_ns"`
	PID         int                `json:"pid"`
	GoVersion   string             `json:"go_version"`
	GOMAXPROCS  int                `json:"gomaxprocs"`
	Goroutines  int                `json:"goroutines"`
	Total       uint64             `json:"total_requests"`
	Active      int                `json:"active_requests"`
	StatusCount map[string]int     `json:"status_counts"`
	StateTime   []StateDuration    `json:"state_time"`
	Memory      MemorySnapshot     `json:"memory"`
	Pool        PoolEstimate       `json:"pool_estimate"`
	Requests    []Request          `json:"requests"`
	Log         []Request          `json:"log"`
	Statistics  StatisticsSnapshot `json:"statistics"`

	StyleCSS template.CSS `json:"-"`
}

// ServerStatus tracks requests, observes PHP runtimes, and serves status data.
// Use Middleware with routers that accept func(http.Handler) http.Handler. The
// type also implements http.Handler for explicitly mounting the status route.
type ServerStatus struct {
	mu        sync.RWMutex
	started   time.Time
	active    map[string]*Request
	history   []Request
	total     uint64
	samples   uint64
	allocated uint64
	stateTime map[runner.Status]time.Duration
}

// NewServerStatus creates an empty process list.
func NewServerStatus() *ServerStatus {
	return &ServerStatus{
		started: time.Now(), active: make(map[string]*Request),
		stateTime: make(map[runner.Status]time.Duration),
	}
}

// Middleware records requests and intercepts ServerStatusPath.
func (s *ServerStatus) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := newULID(time.Now())
		if err != nil {
			http.Error(w, "could not generate request id", http.StatusInternalServerError)
			return
		}
		r.Header.Set("Request-Id", id)
		w.Header().Set("Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)

		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		now := time.Now()
		entry := &Request{
			ID: id, Status: runner.StatusReading, Request: r.Method + " " + r.URL.RequestURI(),
			Hostname: r.Host, Method: r.Method, URI: r.URL.RequestURI(),
			Protocol: r.Proto, RemoteAddress: r.RemoteAddr, UserAgent: r.UserAgent(),
			StartedAt: now, UpdatedAt: now, mem: before, stateChangedAt: now,
		}
		ctx = withRequest(ctx, entry)
		r = r.WithContext(ctx)
		Span(ctx, r.URL.Path, SpanType.HTTP, OpenSpan)
		s.mu.Lock()
		s.total++
		s.active[id] = entry
		s.mu.Unlock()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK, onWrite: func() {
			s.UpdateStatus(ctx, runner.StatusWriting)
		}}
		defer s.finish(ctx, entry, rw)
		if isServerStatusPath(r.URL.Path) {
			s.ServeHTTP(rw, r)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func isServerStatusPath(path string) bool {
	if strings.HasPrefix(path, ServerStatusDetailPath) {
		return true
	}
	switch path {
	case ServerStatusPath, ServerStatusLivePath, ServerStatusLogPath, ServerStatusStatsPath:
		return true
	default:
		return false
	}
}

// UpdateStatus implements runner.Observer.
func (s *ServerStatus) UpdateStatus(ctx context.Context, status runner.Status) {
	id := RequestID(ctx)
	if id == "" {
		return
	}
	s.mu.Lock()
	if entry := s.active[id]; entry != nil {
		now := time.Now()
		s.stateTime[entry.Status] += now.Sub(entry.stateChangedAt)
		entry.Status = status
		entry.UpdatedAt = now
		entry.stateChangedAt = now
	}
	s.mu.Unlock()
}

// UpdateFilename records the PHP entrypoint selected for an active request.
func (s *ServerStatus) UpdateFilename(ctx context.Context, filename string) {
	id := RequestID(ctx)
	if id == "" {
		return
	}
	s.mu.Lock()
	if entry := s.active[id]; entry != nil {
		entry.Filename = filename
		entry.UpdatedAt = time.Now()
	}
	s.mu.Unlock()
	Span(ctx, filename)
}

// UpdateIncludedFiles records the number of files included by an active PHP
// request.
func (s *ServerStatus) UpdateIncludedFiles(ctx context.Context, count int) {
	id := RequestID(ctx)
	if id == "" {
		return
	}
	s.mu.Lock()
	if entry := s.active[id]; entry != nil {
		entry.IncludedFiles = count
		entry.UpdatedAt = time.Now()
	}
	s.mu.Unlock()
}

func (s *ServerStatus) finish(ctx context.Context, entry *Request, rw *responseWriter) {
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	message := entry.Filename
	if message == "" {
		message = entry.URI
	}
	Span(ctx, message, SpanType.HTTP, CloseSpan)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, entry.ID)
	s.stateTime[entry.Status] += now.Sub(entry.stateChangedAt)
	entry.UpdatedAt = now
	entry.Duration = now.Sub(entry.StartedAt)
	entry.ResponseStatus = rw.status
	entry.ResponseBytes = rw.bytes
	entry.HeapDelta = signedDelta(after.HeapAlloc, entry.mem.HeapAlloc)
	entry.AllocatedBytes = delta(after.TotalAlloc, entry.mem.TotalAlloc)
	entry.Allocations = delta(after.Mallocs, entry.mem.Mallocs)
	entry.GCCycles = uint32(delta(uint64(after.NumGC), uint64(entry.mem.NumGC)))
	entry.GCPause = time.Duration(delta(after.PauseTotalNs, entry.mem.PauseTotalNs))
	entry.mem = runtime.MemStats{}
	entry.stateChangedAt = time.Time{}
	s.samples++
	s.allocated += entry.AllocatedBytes
	s.history = append(s.history, *entry)
	if len(s.history) > historyLimit {
		s.history = append(s.history[:0], s.history[len(s.history)-historyLimit:]...)
	}
}

func delta(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func signedDelta(after, before uint64) int64 {
	if after >= before {
		d := after - before
		if d > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(d)
	}
	d := before - after
	if d > math.MaxInt64 {
		return math.MinInt64
	}
	return -int64(d)
}

// Snapshot returns a race-free point-in-time process list.
func (s *ServerStatus) Snapshot() Snapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	s.mu.RLock()
	now := time.Now()
	stateTime := make(map[runner.Status]time.Duration, len(s.stateTime))
	for status, duration := range s.stateTime {
		stateTime[status] = duration
	}
	requests := make([]Request, 0, len(s.active))
	for _, entry := range s.active {
		copy := activeRequest(entry)
		copy.Duration = now.Sub(copy.StartedAt)
		requests = append(requests, copy)
		stateTime[entry.Status] += now.Sub(entry.stateChangedAt)
	}
	history := append([]Request(nil), s.history...)
	total, samples, allocated := s.total, s.samples, s.allocated
	started := s.started
	active := len(s.active)
	s.mu.RUnlock()

	sort.Slice(requests, func(i, j int) bool { return requests[i].StartedAt.After(requests[j].StartedAt) })
	log := make([]Request, len(history))
	for i := range history {
		log[len(history)-1-i] = history[i]
	}
	counts := make(map[string]int)
	for _, request := range requests {
		counts[string(request.Status)]++
	}
	statistics := requestStatistics(history)
	stateDurations := requestStateDurations(stateTime)
	limit := memoryLimit()
	pool := PoolEstimate{Samples: samples}
	if samples > 0 {
		pool.AverageAllocatedBytes = allocated / samples
		if pool.AverageAllocatedBytes > 0 {
			if mem.NextGC > mem.HeapAlloc {
				pool.BeforeNextGC = (mem.NextGC - mem.HeapAlloc) / pool.AverageAllocatedBytes
			}
			if limit > mem.Sys {
				pool.WithinMemoryLimit = (limit - mem.Sys) / pool.AverageAllocatedBytes
			}
		}
	}
	return Snapshot{
		StartedAt:   started,
		Uptime:      time.Since(started),
		PID:         os.Getpid(),
		GoVersion:   runtime.Version(),
		GOMAXPROCS:  runtime.GOMAXPROCS(0),
		Goroutines:  runtime.NumGoroutine(),
		Total:       total,
		Active:      active,
		StatusCount: counts,
		StateTime:   stateDurations,
		Memory: MemorySnapshot{
			HeapAlloc:     mem.HeapAlloc,
			HeapInuse:     mem.HeapInuse,
			HeapObjects:   mem.HeapObjects,
			StackInuse:    mem.StackInuse,
			System:        mem.Sys,
			NextGC:        mem.NextGC,
			NumGC:         mem.NumGC,
			GCPauseTotal:  mem.PauseTotalNs,
			GCCPUFraction: mem.GCCPUFraction,
			MemoryLimit:   limit,
		},
		Pool:       pool,
		Requests:   requests,
		Log:        log,
		Statistics: statistics,
		StyleCSS:   template.CSS(statusCSS),
	}
}

func activeRequest(entry *Request) Request {
	return Request{
		ID:             entry.ID,
		Status:         entry.Status,
		Request:        entry.Request,
		Hostname:       entry.Hostname,
		Filename:       entry.Filename,
		IncludedFiles:  entry.IncludedFiles,
		Method:         entry.Method,
		URI:            entry.URI,
		Protocol:       entry.Protocol,
		RemoteAddress:  entry.RemoteAddress,
		UserAgent:      entry.UserAgent,
		StartedAt:      entry.StartedAt,
		UpdatedAt:      entry.UpdatedAt,
		Duration:       entry.Duration,
		ResponseStatus: entry.ResponseStatus,
		ResponseBytes:  entry.ResponseBytes,
		HeapDelta:      entry.HeapDelta,
		AllocatedBytes: entry.AllocatedBytes,
		Allocations:    entry.Allocations,
		GCCycles:       entry.GCCycles,
		GCPause:        entry.GCPause,
	}
}

func requestStateDurations(durations map[runner.Status]time.Duration) []StateDuration {
	states := []struct {
		status runner.Status
		label  string
	}{
		{runner.StatusWaiting, "Waiting"},
		{runner.StatusStarting, "Starting"},
		{runner.StatusReading, "Reading"},
		{runner.StatusProcessing, "Processing"},
		{runner.StatusWriting, "Writing"},
		{runner.StatusKeepalive, "Keepalive"},
		{runner.StatusClosing, "Closing"},
		{runner.StatusError, "Error"},
	}
	var total time.Duration
	for _, duration := range durations {
		total += duration
	}
	result := make([]StateDuration, 0, len(states))
	for _, state := range states {
		duration := durations[state.status]
		if duration <= 0 {
			continue
		}
		result = append(result, StateDuration{
			State: state.status, Label: state.label, Duration: duration,
			Share: float64(duration) * 100 / float64(total),
		})
	}
	return result
}

func requestStatistics(history []Request) StatisticsSnapshot {
	result := StatisticsSnapshot{WindowSize: len(history), WindowLimit: historyLimit, TopLimit: topLimit}
	if len(history) == 0 {
		return result
	}
	grouped := make(map[string]*RequestStatistic)
	for _, request := range history {
		key := request.Hostname + "\x00" + request.Request + "\x00" + request.Filename
		stat := grouped[key]
		if stat == nil {
			stat = &RequestStatistic{Request: request.Request, Hostname: request.Hostname, Filename: request.Filename}
			grouped[key] = stat
		}
		stat.Count++
		stat.totalDuration += request.Duration
		if request.ResponseBytes > 0 {
			stat.totalResponseBytes += uint64(request.ResponseBytes)
		}
		stat.totalAllocated += request.AllocatedBytes
		stat.totalIncluded += uint64(request.IncludedFiles)
	}
	result.Top = make([]RequestStatistic, 0, len(grouped))
	for _, stat := range grouped {
		stat.Share = float64(stat.Count) * 100 / float64(len(history))
		stat.AverageDuration = stat.totalDuration / time.Duration(stat.Count)
		stat.AverageResponseBytes = stat.totalResponseBytes / stat.Count
		stat.AverageAllocatedBytes = stat.totalAllocated / stat.Count
		stat.AverageIncludedFiles = float64(stat.totalIncluded) / float64(stat.Count)
		result.Top = append(result.Top, *stat)
	}
	sort.Slice(result.Top, func(i, j int) bool {
		if result.Top[i].Count != result.Top[j].Count {
			return result.Top[i].Count > result.Top[j].Count
		}
		if result.Top[i].totalDuration != result.Top[j].totalDuration {
			return result.Top[i].totalDuration > result.Top[j].totalDuration
		}
		if result.Top[i].Request != result.Top[j].Request {
			return result.Top[i].Request < result.Top[j].Request
		}
		if result.Top[i].Hostname != result.Top[j].Hostname {
			return result.Top[i].Hostname < result.Top[j].Hostname
		}
		return result.Top[i].Filename < result.Top[j].Filename
	})
	if len(result.Top) > topLimit {
		result.Top = result.Top[:topLimit]
	}
	return result
}

func memoryLimit() uint64 {
	var limits []uint64
	if limit := debug.SetMemoryLimit(-1); limit > 0 && limit != math.MaxInt64 {
		limits = append(limits, uint64(limit))
	}
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		value, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(value)) == "max" {
			continue
		}
		if limit, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64); err == nil && limit > 0 {
			limits = append(limits, limit)
		}
	}
	if value, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(value), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				if kib, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					limits = append(limits, kib*1024)
				}
				break
			}
		}
	}
	if len(limits) == 0 {
		return 0
	}
	return slices.Min(limits)
}

type statusView string

const (
	statusViewOverview statusView = "overview"
	statusViewLive     statusView = "live"
	statusViewLog      statusView = "log"
	statusViewStats    statusView = "stats"
	statusViewDetail   statusView = "detail"
)

type statusPage struct {
	Snapshot
	View     string
	Limit    int
	Detail   *Request
	SpanRows []spanRow
}

type spanRow struct {
	RequestSpan
	Offset      time.Duration
	Duration    time.Duration
	HasDuration bool
	Depth       int
}

// ServeHTTP renders one server-status view as JSON, plain text, or HTML.
func (s *ServerStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if id, ok := strings.CutPrefix(r.URL.Path, ServerStatusDetailPath); ok {
		s.serveDetail(w, r, id)
		return
	}
	view, ok := viewForPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	snapshot := s.Snapshot()
	limit := requestedLogLimit(r)
	if view == statusViewLog && len(snapshot.Log) > limit {
		snapshot.Log = snapshot.Log[:limit]
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "text/json") || strings.Contains(accept, "application/json"):
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		writeJSON(w, snapshot, view)
	case strings.Contains(accept, "text/plain") || strings.HasPrefix(strings.ToLower(r.UserAgent()), "curl/"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeText(w, snapshot, view)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = statusTemplate.Execute(w, statusPage{Snapshot: snapshot, View: string(view), Limit: limit})
	}
}

func (s *ServerStatus) serveDetail(w http.ResponseWriter, r *http.Request, id string) {
	request, ok := s.completedRequest(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	rows := requestSpanRows(request)
	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "text/json") || strings.Contains(accept, "application/json"):
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(request)
	case strings.Contains(accept, "text/plain") || strings.HasPrefix(strings.ToLower(r.UserAgent()), "curl/"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeDetailText(w, request, rows)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = statusTemplate.Execute(w, statusPage{Snapshot: s.Snapshot(), View: string(statusViewDetail), Detail: &request, SpanRows: rows})
	}
}

func (s *ServerStatus) completedRequest(id string) (Request, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].ID == id {
			return s.history[i], true
		}
	}
	return Request{}, false
}

func requestSpanRows(request Request) []spanRow {
	rows := make([]spanRow, len(request.Spans))
	depth := 0
	for i, span := range request.Spans {
		if span.Close && depth > 0 {
			depth--
		}
		rows[i] = spanRow{RequestSpan: span, Offset: span.Time.Sub(request.StartedAt), Depth: depth}
		if i+1 < len(request.Spans) {
			rows[i].Duration = request.Spans[i+1].Time.Sub(span.Time)
			rows[i].HasDuration = true
		}
		if span.Open {
			depth++
		}
	}
	return rows
}

func requestedLogLimit(r *http.Request) int {
	switch r.URL.Query().Get("limit") {
	case "50":
		return 50
	case "100":
		return 100
	default:
		return 20
	}
}

func viewForPath(path string) (statusView, bool) {
	switch path {
	case ServerStatusPath:
		return statusViewOverview, true
	case ServerStatusLivePath:
		return statusViewLive, true
	case ServerStatusLogPath:
		return statusViewLog, true
	case ServerStatusStatsPath:
		return statusViewStats, true
	default:
		return "", false
	}
}

func writeJSON(w io.Writer, snapshot Snapshot, view statusView) {
	encoder := json.NewEncoder(w)
	switch view {
	case statusViewLog:
		_ = encoder.Encode(snapshot.Log)
	case statusViewStats:
		_ = encoder.Encode(snapshot.Statistics)
	default:
		_ = encoder.Encode(snapshot)
	}
}

type responseWriter struct {
	http.ResponseWriter
	status  int
	bytes   int64
	onWrite func()
	wrote   bool
}

// Unwrap lets http.ResponseController reach optional interfaces implemented by
// the original response writer.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status
	w.onWrite()
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *responseWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeText(w io.Writer, s Snapshot, view statusView) {
	fmt.Fprintf(w, "phpscript server status\nUptime: %s  PID: %d  Go: %s  GOMAXPROCS: %d  Goroutines: %d\n", s.Uptime.Round(time.Millisecond), s.PID, s.GoVersion, s.GOMAXPROCS, s.Goroutines)
	fmt.Fprintf(w, "Requests: %d total, %d active  Heap: %s / next GC %s  GC: %d cycles, %.2f%% CPU\n", s.Total, s.Active, bytesText(s.Memory.HeapAlloc), bytesText(s.Memory.NextGC), s.Memory.NumGC, s.Memory.GCCPUFraction*100)
	fmt.Fprintf(w, "Pool estimate: %d samples, %s/request, %d before GC, %d within memory limit\n\n", s.Pool.Samples, bytesText(s.Pool.AverageAllocatedBytes), s.Pool.BeforeNextGC, s.Pool.WithinMemoryLimit)
	switch view {
	case statusViewOverview:
		writeRequestLogText(w, s.Log)
	case statusViewLog:
		writeRequestLogText(w, s.Log)
	case statusViewStats:
		fmt.Fprintf(w, "Top requests (last %d of %d):\n", s.Statistics.WindowSize, s.Statistics.WindowLimit)
		fmt.Fprintln(w, "SHARE    COUNT  AVG TIME      AVG BYTES  AVG ALLOC  AVG INCLUDED  REQUEST                         HOSTNAME                 FILENAME")
		for _, stat := range s.Statistics.Top {
			fmt.Fprintf(w, "%6.2f%%  %-5d  %-12s  %-9s  %-9s  %-12s  %-31s %-24s %s\n", stat.Share, stat.Count, stat.AverageDuration.Round(time.Microsecond), bytesText(stat.AverageResponseBytes), bytesText(stat.AverageAllocatedBytes), averageIncludedText(stat.AverageIncludedFiles), stat.Request, stat.Hostname, stat.Filename)
		}
	default:
		fmt.Fprintln(w, "S  REQUEST-ID                  REQUEST                         HOSTNAME                 FILENAME                       INCLUDED          STATUS  DURATION     BYTES      HEAP       ALLOC      OBJECTS    GC/PAUSE       REMOTE")
		for _, request := range s.Requests {
			fmt.Fprintf(w, "%-2s %-26s %-31s %-24s %-30s %-17s %-7d %-12s %-10d %-10s %-10s %-10d %d/%-12s %s\n", request.Status, request.ID, request.Request, request.Hostname, request.Filename, includedText(request.IncludedFiles), request.ResponseStatus, request.Duration.Round(time.Microsecond), request.ResponseBytes, signedBytesText(request.HeapDelta), bytesText(request.AllocatedBytes), request.Allocations, request.GCCycles, request.GCPause.Round(time.Microsecond), request.RemoteAddress)
		}
		fmt.Fprintln(w, "\nLifetime request state time:")
		for _, state := range s.StateTime {
			fmt.Fprintf(w, "%s (%s): %s, %.2f%%\n", state.Label, state.State, state.Duration.Round(time.Microsecond), state.Share)
		}
	}
}

func writeRequestLogText(w io.Writer, requests []Request) {
	fmt.Fprintf(w, "Request log (latest %d):\n", len(requests))
	fmt.Fprintln(w, "REQUEST-ID                  REQUEST                         HOSTNAME                 FILENAME                       INCLUDED          STATUS  DURATION     BYTES      HEAP       ALLOC      OBJECTS    GC/PAUSE")
	for _, request := range requests {
		fmt.Fprintf(w, "%-26s %-31s %-24s %-30s %-17s %-7d %-12s %-10d %-10s %-10s %-10d %d/%s\n", request.ID, request.Request, request.Hostname, request.Filename, includedText(request.IncludedFiles), request.ResponseStatus, request.Duration.Round(time.Microsecond), request.ResponseBytes, signedBytesText(request.HeapDelta), bytesText(request.AllocatedBytes), request.Allocations, request.GCCycles, request.GCPause.Round(time.Microsecond))
	}
}

func writeDetailText(w io.Writer, request Request, rows []spanRow) {
	fmt.Fprintf(w, "%s\nRequest ID: %s  Status: %d  Duration: %s\n\n", request.Request, request.ID, request.ResponseStatus, request.Duration.Round(time.Microsecond))
	fmt.Fprintln(w, "TYPE          TIME          DURATION      MESSAGE")
	for _, row := range rows {
		duration := "-"
		if row.HasDuration {
			duration = row.Duration.Round(time.Microsecond).String()
		}
		fmt.Fprintf(w, "%-13s %-13s %-13s %s%s\n", row.Type, row.Offset.Round(time.Microsecond), duration, strings.Repeat("  ", row.Depth), row.Message)
	}
}

func includedText(count int) string {
	if count == 0 {
		return "/"
	}
	return strconv.Itoa(count)
}

func averageIncludedText(count float64) string {
	if count == 0 {
		return "/"
	}
	return fmt.Sprintf("%.1f", count)
}

func signedBytesText(n int64) string {
	if n < 0 {
		if n == math.MinInt64 {
			return "-" + bytesText(uint64(math.MaxInt64)+1)
		}
		return "-" + bytesText(uint64(-n))
	}
	return bytesText(uint64(n))
}

func bytesText(n uint64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatUint(n, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func spanTypeColor(spanType Flag) string {
	switch spanType {
	case SpanType.Database:
		return "#2563eb"
	case SpanType.Internal:
		return "#6b7280"
	case SpanType.External:
		return "#7c3aed"
	case SpanType.Template:
		return "#db2777"
	case SpanType.Cache:
		return "#0891b2"
	case SpanType.HTTP:
		return "#16a34a"
	default:
		return "#d97706"
	}
}

var statusTemplate = template.Must(template.New("server-status").Funcs(template.FuncMap{
	"duration":        func(d time.Duration) string { return d.Round(time.Microsecond).String() },
	"included":        includedText,
	"averageIncluded": averageIncludedText,
	"stateClass":      func(label string) string { return strings.ToLower(label) },
	"stateWidth": func(share float64) template.CSS {
		return template.CSS(fmt.Sprintf("width:%.4f%%", share))
	},
	"spanIndent": func(depth int) template.CSS {
		return template.CSS(fmt.Sprintf("margin-left:%.1fem", float64(depth)*1.5))
	},
	"spanColor": func(depth int) template.CSS {
		depth = min(depth, 4)
		red := 255 - depth*10/4
		green := 255 - depth*97/4
		blue := 255 - depth*244/4
		return template.CSS(fmt.Sprintf("background:rgb(%d,%d,%d)", red, green, blue))
	},
	"spanTypeStyle": func(spanType Flag) template.CSS {
		color := spanTypeColor(spanType)
		return template.CSS(fmt.Sprintf("background:%s;color:white", color))
	},
	"spanBorder": func(spanType Flag) template.CSS {
		return template.CSS("border-left:4px solid " + spanTypeColor(spanType))
	},
	"bytes": func(value any) string {
		switch n := value.(type) {
		case uint64:
			return bytesText(n)
		case int64:
			return signedBytesText(n)
		default:
			return "0 B"
		}
	},
}).Parse(statusTemplateContents))

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newULID(now time.Time) (string, error) {
	var data [16]byte
	ms := uint64(now.UnixMilli())
	data[0], data[1], data[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	data[3], data[4], data[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	if _, err := rand.Read(data[6:]); err != nil {
		return "", err
	}
	var out [26]byte
	for i := range out {
		var value byte
		for bit := 0; bit < 5; bit++ {
			streamBit := i*5 + bit - 2
			value <<= 1
			if streamBit >= 0 && streamBit < 128 {
				value |= (data[streamBit/8] >> (7 - streamBit%8)) & 1
			}
		}
		out[i] = crockford[value]
	}
	return string(out[:]), nil
}
