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

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/model"
)

// ServerStatusPath is the default status overview served by ServerStatus.
const ServerStatusPath = "/debug/server-status"

const (
	ServerStatusLivePath   = ServerStatusPath + "/live"
	ServerStatusLogPath    = ServerStatusPath + "/log"
	ServerStatusStatsPath  = ServerStatusPath + "/stats"
	ServerStatusDetailPath = ServerStatusPath + "/detail/"
)

type (
	Request          = model.Request
	RequestStatistic = model.RequestStatistic
	RequestSpan      = model.RequestSpan
	Flag             = model.Flag
)

var (
	StartSpan = model.StartSpan
	OpenSpan  = model.OpenSpan
	CloseSpan = model.CloseSpan
	SpanType  = model.SpanType
)

type requestIDKey struct{}

// RequestID returns the ULID assigned to the request in ctx.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
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
	State    model.Status  `json:"state"`
	Label    string        `json:"label"`
	Duration time.Duration `json:"duration_ns"`
	Share    float64       `json:"share_percent"`
}

// SpanDuration is one point-in-time segment where a span type was the active,
// innermost region of a request.
type SpanDuration struct {
	Type        Flag          `json:"type"`
	Offset      time.Duration `json:"offset_ns"`
	Duration    time.Duration `json:"duration_ns"`
	OffsetShare float64       `json:"offset_percent"`
	Share       float64       `json:"share_percent"`
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
	platform.UnimplementedModule

	mu        sync.RWMutex
	started   time.Time
	active    map[string]*Request
	history   []Request
	total     uint64
	samples   uint64
	allocated uint64
	stateTime map[model.Status]time.Duration
	options   Options
}

var _ platform.Module = (*ServerStatus)(nil)

var _ model.Tracer = (*ServerStatus)(nil)

// NewModule creates an empty status module.
func NewModule(options Options) *ServerStatus {
	return &ServerStatus{
		UnimplementedModule: *platform.NewUnimplementedModule("status"),
		started:             time.Now(),
		active:              make(map[string]*Request),
		stateTime:           make(map[model.Status]time.Duration),
		options:             options,
	}
}

// NewServerStatus creates an empty status module.
func NewServerStatus(options Options) *ServerStatus {
	return NewModule(options)
}

// Mount registers status page routes on the platform router.
func (s *ServerStatus) Mount(_ context.Context, r platform.Router) error {
	r.Handle(ServerStatusPath, s)
	r.Handle(ServerStatusLivePath, s)
	r.Handle(ServerStatusLogPath, s)
	r.Handle(ServerStatusStatsPath, s)
	r.Handle(ServerStatusDetailPath+"*", s)
	return nil
}

// Middleware records requests handled by the platform.
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
		if s.options.TrackMemoryUse {
			runtime.ReadMemStats(&before)
		}
		now := time.Now()
		entry := &Request{
			ID:            id,
			Status:        model.StatusReading,
			Request:       r.Method + " " + r.URL.RequestURI(),
			Hostname:      r.Host,
			Method:        r.Method,
			URI:           r.URL.RequestURI(),
			Protocol:      r.Proto,
			RemoteAddress: r.RemoteAddr,
			UserAgent:     r.UserAgent(),
			StartedAt:     now,
			UpdatedAt:     now,
			MemStats:      before,
			ChangedAt:     now,
		}
		ctx = model.WithRequest(ctx, entry)
		r = r.WithContext(ctx)
		model.StartSpan(ctx, r.URL.Path, model.SpanType.HTTP, model.OpenSpan)
		s.mu.Lock()
		s.total++
		s.active[id] = entry
		s.mu.Unlock()

		rw := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
			onWrite: func() {
				s.UpdateStatus(ctx, model.StatusWriting)
			},
		}
		defer s.finish(ctx, entry, rw)
		next.ServeHTTP(rw, r)
	})
}

// UpdateStatus implements runner.Observer.
func (s *ServerStatus) UpdateStatus(ctx context.Context, status model.Status) {
	id := RequestID(ctx)
	if id == "" {
		return
	}
	s.mu.Lock()
	if entry := s.active[id]; entry != nil {
		now := time.Now()
		s.stateTime[entry.Status] += now.Sub(entry.ChangedAt)
		entry.Status = status
		entry.UpdatedAt = now
		entry.ChangedAt = now
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
		for _, span := range entry.Spans {
			if span != nil && span.Filename == "" {
				span.SetFilename(filename)
			}
		}
	}
	s.mu.Unlock()
	model.StartSpan(model.WithSpanFilename(ctx, filename), filename, model.OpenSpan)
}

// StartSpan starts a named span for ctx.
func (s *ServerStatus) StartSpan(ctx context.Context, name string) model.Span {
	return model.StartSpan(ctx, name)
}

// Trace will add a span with the invoked message. It's used to trace
// runtime internals, like including files.
func (s *ServerStatus) Trace(ctx context.Context, message string, flags ...model.Flag) *model.RequestSpan {
	return model.StartSpan(ctx, message, flags...)
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
	if s.options.TrackMemoryUse {
		runtime.ReadMemStats(&after)
	}
	model.StartSpan(model.WithSpanFilename(ctx, entry.Filename), "done", model.SpanType.HTTP, model.CloseSpan)

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, entry.ID)
	s.stateTime[entry.Status] += now.Sub(entry.ChangedAt)
	entry.UpdatedAt = now
	entry.Duration = now.Sub(entry.StartedAt)
	entry.ResponseStatus = rw.status
	entry.ResponseBytes = rw.bytes
	entry.HeapDelta = signedDelta(after.HeapAlloc, entry.MemStats.HeapAlloc)
	entry.AllocatedBytes = delta(after.TotalAlloc, entry.MemStats.TotalAlloc)
	entry.Allocations = delta(after.Mallocs, entry.MemStats.Mallocs)
	entry.GCCycles = uint32(delta(uint64(after.NumGC), uint64(entry.MemStats.NumGC)))
	entry.GCPause = time.Duration(delta(after.PauseTotalNs, entry.MemStats.PauseTotalNs))
	entry.MemStats = runtime.MemStats{}
	entry.ChangedAt = time.Time{}
	s.samples++
	s.allocated += entry.AllocatedBytes
	s.history = append(s.history, *entry)
	if len(s.history) > s.options.RingBufferSize {
		s.history = append(s.history[:0], s.history[len(s.history)-s.options.RingBufferSize:]...)
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
	stateTime := make(map[model.Status]time.Duration, len(s.stateTime))
	for status, duration := range s.stateTime {
		stateTime[status] = duration
	}
	requests := make([]Request, 0, len(s.active))
	for _, entry := range s.active {
		copy := activeRequest(entry)
		copy.Duration = now.Sub(copy.StartedAt)
		requests = append(requests, copy)
		stateTime[entry.Status] += now.Sub(entry.ChangedAt)
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
	statistics := requestStatistics(history, s.options)
	stateDurations := requestStateDurations(stateTime)
	limit := memoryLimit()
	pool := PoolEstimate{
		Samples: samples,
	}
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

func requestStateDurations(durations map[model.Status]time.Duration) []StateDuration {
	states := []struct {
		status model.Status
		label  string
	}{
		{
			status: model.StatusWaiting,
			label:  "Waiting",
		},
		{
			status: model.StatusStarting,
			label:  "Starting",
		},
		{
			status: model.StatusReading,
			label:  "Reading",
		},
		{
			status: model.StatusProcessing,
			label:  "Processing",
		},
		{
			status: model.StatusWriting,
			label:  "Writing",
		},
		{
			status: model.StatusKeepalive,
			label:  "Keepalive",
		},
		{
			status: model.StatusClosing,
			label:  "Closing",
		},
		{
			status: model.StatusError,
			label:  "Error",
		},
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
			State:    state.status,
			Label:    state.label,
			Duration: duration,
			Share:    float64(duration) * 100 / float64(total),
		})
	}
	return result
}

func requestStatistics(history []Request, options Options) StatisticsSnapshot {
	result := StatisticsSnapshot{
		WindowSize:  len(history),
		WindowLimit: options.RingBufferSize,
		TopLimit:    options.TopRequests,
	}
	if len(history) == 0 {
		return result
	}
	grouped := make(map[string]*RequestStatistic)
	for _, request := range history {
		key := request.Hostname + "\x00" + request.Request + "\x00" + request.Filename
		stat := grouped[key]
		if stat == nil {
			stat = &RequestStatistic{
				Request:  request.Request,
				Hostname: request.Hostname,
				Filename: request.Filename,
			}
			grouped[key] = stat
		}
		stat.Count++
		stat.TotalDuration += request.Duration
		if request.ResponseBytes > 0 {
			stat.TotalResponseBytes += uint64(request.ResponseBytes)
		}
		stat.TotalAllocated += request.AllocatedBytes
		stat.TotalIncluded += uint64(request.IncludedFiles)
	}
	result.Top = make([]RequestStatistic, 0, len(grouped))
	for _, stat := range grouped {
		stat.Share = float64(stat.Count) * 100 / float64(len(history))
		stat.AverageDuration = stat.TotalDuration / time.Duration(stat.Count)
		stat.AverageResponseBytes = stat.TotalResponseBytes / stat.Count
		stat.AverageAllocatedBytes = stat.TotalAllocated / stat.Count
		stat.AverageIncludedFiles = float64(stat.TotalIncluded) / float64(stat.Count)
		result.Top = append(result.Top, *stat)
	}
	sort.Slice(result.Top, func(i, j int) bool {
		if result.Top[i].Count != result.Top[j].Count {
			return result.Top[i].Count > result.Top[j].Count
		}
		if result.Top[i].TotalDuration != result.Top[j].TotalDuration {
			return result.Top[i].TotalDuration > result.Top[j].TotalDuration
		}
		if result.Top[i].Request != result.Top[j].Request {
			return result.Top[i].Request < result.Top[j].Request
		}
		if result.Top[i].Hostname != result.Top[j].Hostname {
			return result.Top[i].Hostname < result.Top[j].Hostname
		}
		return result.Top[i].Filename < result.Top[j].Filename
	})
	if len(result.Top) > options.TopRequests {
		result.Top = result.Top[:options.TopRequests]
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
	SpanTime []SpanDuration
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
		_ = statusTemplate.Execute(w, statusPage{
			Snapshot: snapshot,
			View:     string(view),
			Limit:    limit,
		})
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
		_ = statusTemplate.Execute(w, statusPage{
			Snapshot: s.Snapshot(),
			View:     string(statusViewDetail),
			Detail:   &request,
			SpanRows: rows,
			SpanTime: requestSpanDurations(request),
		})
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
	stack := make([]int, 0)
	for i, span := range request.Spans {
		if span == nil {
			continue
		}
		if span.Close {
			if open := matchingOpenSpan(stack, rows, span.Type); open >= 0 {
				rows[open].Duration = span.Time.Sub(rows[open].Time)
				rows[open].HasDuration = rows[open].Duration >= 0
				stack = slices.Delete(stack, slices.Index(stack, open), slices.Index(stack, open)+1)
			}
		}
		rows[i] = spanRow{
			RequestSpan: *span,
			Offset:      span.Time.Sub(request.StartedAt),
			Duration:    span.Duration,
			HasDuration: span.Duration > 0,
			Depth:       len(stack),
		}
		if span.Open {
			stack = append(stack, i)
		}
	}
	return rows
}

func matchingOpenSpan(stack []int, rows []spanRow, spanType Flag) int {
	for i := len(stack) - 1; i >= 0; i-- {
		if rows[stack[i]].Type == spanType {
			return stack[i]
		}
	}
	return -1
}

func requestSpanDurations(request Request) []SpanDuration {
	rows := requestSpanRows(request)
	if request.Duration <= 0 {
		return nil
	}
	requestEnd := request.StartedAt.Add(request.Duration)
	type interval struct {
		spanType Flag
		start    time.Time
		end      time.Time
	}
	intervals := make([]interval, 0, len(rows))
	boundaries := make([]time.Time, 0, len(rows)*2)
	for _, row := range rows {
		if !row.HasDuration || row.Duration <= 0 {
			continue
		}
		if row.Type == SpanType.HTTP && row.Depth == 0 {
			continue
		}
		start, end := row.Time, row.Time.Add(row.Duration)
		if start.Before(request.StartedAt) {
			start = request.StartedAt
		}
		if end.After(requestEnd) {
			end = requestEnd
		}
		if !end.After(start) {
			continue
		}
		intervals = append(intervals, interval{
			spanType: row.Type,
			start:    start,
			end:      end,
		})
		boundaries = append(boundaries, start, end)
	}
	slices.SortFunc(boundaries, func(a, b time.Time) int { return a.Compare(b) })
	boundaries = slices.Compact(boundaries)

	result := make([]SpanDuration, 0, len(boundaries))
	for i := 0; i+1 < len(boundaries); i++ {
		start, end := boundaries[i], boundaries[i+1]
		active := -1
		for j, candidate := range intervals {
			if candidate.start.After(start) || !candidate.end.After(start) {
				continue
			}
			if active < 0 || candidate.start.After(intervals[active].start) || candidate.start.Equal(intervals[active].start) && candidate.end.Before(intervals[active].end) {
				active = j
			}
		}
		if active < 0 {
			continue
		}
		spanType, duration := intervals[active].spanType, end.Sub(start)
		if len(result) > 0 {
			previous := &result[len(result)-1]
			if previous.Type == spanType && previous.Offset+previous.Duration == start.Sub(request.StartedAt) {
				previous.Duration += duration
				previous.Share = float64(previous.Duration) * 100 / float64(request.Duration)
				continue
			}
		}
		offset := start.Sub(request.StartedAt)
		result = append(result, SpanDuration{
			Type:        spanType,
			Offset:      offset,
			Duration:    duration,
			OffsetShare: float64(offset) * 100 / float64(request.Duration),
			Share:       float64(duration) * 100 / float64(request.Duration),
		})
	}
	return result
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
	fmt.Fprintln(w, "Process state:")
	for _, state := range requestSpanDurations(request) {
		fmt.Fprintf(w, "%s at %s: %s, %.2f%%\n", state.Type, state.Offset.Round(time.Microsecond), state.Duration.Round(time.Microsecond), state.Share)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "TYPE          TIME          DURATION      SOURCE                         MESSAGE")
	for _, row := range rows {
		duration := "-"
		if row.HasDuration {
			duration = durationMilliseconds(row.Duration)
		}
		fmt.Fprintf(w, "%-13s %-13s %-13s %-30s %s%s\n", row.Type, row.Offset.Round(time.Microsecond), duration, spanSource(row.Filename, row.Line), strings.Repeat("  ", row.Depth), row.Message)
	}
}

func spanSource(filename string, line int) string {
	if line <= 0 {
		return filename
	}
	if filename == "" {
		return fmt.Sprintf("L%d", line)
	}
	return fmt.Sprintf("%s:L%d", filename, line)
}

func durationMilliseconds(duration time.Duration) string {
	return fmt.Sprintf("%.4f ms", float64(duration)/float64(time.Millisecond))
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
	case model.SpanType.Database:
		return "#2563eb"
	case model.SpanType.Internal:
		return "#6b7280"
	case model.SpanType.External:
		return "#7c3aed"
	case model.SpanType.Template:
		return "#db2777"
	case model.SpanType.Cache:
		return "#0891b2"
	case model.SpanType.HTTP:
		return "#16a34a"
	default:
		return "#d97706"
	}
}

var statusTemplate = template.Must(template.New("server-status").Funcs(template.FuncMap{
	"duration":        func(d time.Duration) string { return d.Round(time.Microsecond).String() },
	"durationMS":      durationMilliseconds,
	"spanSource":      spanSource,
	"addDuration":     func(a, b time.Duration) time.Duration { return a + b },
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
	"spanStateStyle": func(state SpanDuration) template.CSS {
		return template.CSS(fmt.Sprintf("left:%.4f%%;width:%.4f%%;background:%s", state.OffsetShare, state.Share, spanTypeColor(state.Type)))
	},
	"spanTypeBackground": func(spanType Flag) template.CSS {
		return template.CSS("background:" + spanTypeColor(spanType))
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
