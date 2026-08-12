// Package middleware provides HTTP middleware for phpscript services.
package middleware

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

// ServerStatusPath is the default live-process endpoint served by ServerStatus.
const ServerStatusPath = "/debug/server-status"

const (
	ServerStatusLivePath  = ServerStatusPath + "/live"
	ServerStatusLogPath   = ServerStatusPath + "/log"
	ServerStatusStatsPath = ServerStatusPath + "/stats"
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

// RequestSnapshot describes an active or recently completed request.
type RequestSnapshot struct {
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
	Requests    []RequestSnapshot  `json:"requests"`
	Log         []RequestSnapshot  `json:"log"`
	Statistics  StatisticsSnapshot `json:"statistics"`
}

// ServerStatus tracks requests, observes PHP runtimes, and serves status data.
// Use Middleware with routers that accept func(http.Handler) http.Handler. The
// type also implements http.Handler for explicitly mounting the status route.
type ServerStatus struct {
	mu        sync.RWMutex
	started   time.Time
	active    map[string]*RequestSnapshot
	history   []RequestSnapshot
	total     uint64
	samples   uint64
	allocated uint64
	stateTime map[runner.Status]time.Duration
}

// NewServerStatus creates an empty process list.
func NewServerStatus() *ServerStatus {
	return &ServerStatus{
		started: time.Now(), active: make(map[string]*RequestSnapshot),
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
		r = r.WithContext(ctx)

		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		now := time.Now()
		entry := &RequestSnapshot{
			ID: id, Status: runner.StatusReading, Request: r.Method + " " + r.URL.RequestURI(),
			Hostname: r.Host, Method: r.Method, URI: r.URL.RequestURI(),
			Protocol: r.Proto, RemoteAddress: r.RemoteAddr, UserAgent: r.UserAgent(),
			StartedAt: now, UpdatedAt: now, mem: before, stateChangedAt: now,
		}
		s.mu.Lock()
		s.total++
		s.active[id] = entry
		s.mu.Unlock()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK, onWrite: func() {
			s.UpdateStatus(ctx, runner.StatusWriting)
		}}
		defer s.finish(entry, rw)
		if isServerStatusPath(r.URL.Path) {
			s.ServeHTTP(rw, r)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func isServerStatusPath(path string) bool {
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

func (s *ServerStatus) finish(entry *RequestSnapshot, rw *responseWriter) {
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
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
	requests := make([]RequestSnapshot, 0, len(s.active))
	for _, entry := range s.active {
		copy := *entry
		copy.Duration = now.Sub(copy.StartedAt)
		copy.mem = runtime.MemStats{}
		copy.stateChangedAt = time.Time{}
		requests = append(requests, copy)
		stateTime[entry.Status] += now.Sub(entry.stateChangedAt)
	}
	history := append([]RequestSnapshot(nil), s.history...)
	total, samples, allocated := s.total, s.samples, s.allocated
	started := s.started
	active := len(s.active)
	s.mu.RUnlock()

	sort.Slice(requests, func(i, j int) bool { return requests[i].StartedAt.After(requests[j].StartedAt) })
	log := make([]RequestSnapshot, len(history))
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
		StartedAt: started, Uptime: time.Since(started), PID: os.Getpid(),
		GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0), Goroutines: runtime.NumGoroutine(),
		Total: total, Active: active, StatusCount: counts, StateTime: stateDurations,
		Memory: MemorySnapshot{
			HeapAlloc: mem.HeapAlloc, HeapInuse: mem.HeapInuse, HeapObjects: mem.HeapObjects,
			StackInuse: mem.StackInuse, System: mem.Sys, NextGC: mem.NextGC, NumGC: mem.NumGC,
			GCPauseTotal: mem.PauseTotalNs, GCCPUFraction: mem.GCCPUFraction, MemoryLimit: limit,
		},
		Pool: pool, Requests: requests, Log: log, Statistics: statistics,
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

func requestStatistics(history []RequestSnapshot) StatisticsSnapshot {
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
	statusViewLive  statusView = "live"
	statusViewLog   statusView = "log"
	statusViewStats statusView = "stats"
)

type statusPage struct {
	Snapshot
	View  string
	Limit int
}

// ServeHTTP renders one server-status view as JSON, plain text, or HTML.
func (s *ServerStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	case ServerStatusPath, ServerStatusLivePath:
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
	case statusViewLog:
		fmt.Fprintf(w, "Request log (latest %d):\n", len(s.Log))
		fmt.Fprintln(w, "REQUEST                         HOSTNAME                 FILENAME                       INCLUDED          STATUS  DURATION     BYTES      HEAP       ALLOC      OBJECTS    GC/PAUSE")
		for _, request := range s.Log {
			fmt.Fprintf(w, "%-31s %-24s %-30s %-17s %-7d %-12s %-10d %-10s %-10s %-10d %d/%s\n", request.Request, request.Hostname, request.Filename, includedText(request.IncludedFiles), request.ResponseStatus, request.Duration.Round(time.Microsecond), request.ResponseBytes, signedBytesText(request.HeapDelta), bytesText(request.AllocatedBytes), request.Allocations, request.GCCycles, request.GCPause.Round(time.Microsecond))
		}
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

var statusTemplate = template.Must(template.New("server-status").Funcs(template.FuncMap{
	"duration":        func(d time.Duration) string { return d.Round(time.Microsecond).String() },
	"included":        includedText,
	"averageIncluded": averageIncludedText,
	"stateClass":      func(label string) string { return strings.ToLower(label) },
	"stateWidth": func(share float64) template.CSS {
		return template.CSS(fmt.Sprintf("width:%.4f%%", share))
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
}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>phpscript server status</title>
<style>body{font:14px system-ui,sans-serif;margin:2rem;color:#222}table{border-collapse:collapse;width:100%}th,td{padding:.4rem .6rem;border-bottom:1px solid #ddd;text-align:left;white-space:nowrap}th{background:#f3f3f3}.metrics{display:flex;gap:2rem;flex-wrap:wrap}.metrics div{padding:.8rem;background:#f7f7f7}code{font-weight:bold}.tabs{display:flex;gap:.4rem;margin:1.5rem 0 .8rem}.tabs a{border:1px solid #bbb;background:#eee;padding:.5rem .9rem;color:#222;text-decoration:none}.tabs a.active{background:#333;color:#fff}.share meter{width:7rem;margin-right:.5rem}.state-bar{display:flex;height:2rem;width:100%;overflow:hidden;background:#eee;margin:.7rem 0}.state-bar span{min-width:1px}.state-legend{display:flex;flex-wrap:wrap;gap:1rem}.state-legend i{display:inline-block;width:.8rem;height:.8rem;margin-right:.3rem}.waiting{background:#9ca3af}.starting{background:#a78bfa}.reading{background:#60a5fa}.processing{background:#f59e0b}.writing{background:#34d399}.keepalive{background:#22d3ee}.closing{background:#64748b}.error{background:#ef4444}</style></head>
<body><h1>phpscript server status</h1><div class="metrics">
<div><b>Uptime</b><br>{{duration .Uptime}}</div><div><b>Requests</b><br>{{.Total}} total / {{.Active}} active</div>
<div><b>Heap</b><br>{{bytes .Memory.HeapAlloc}} / {{bytes .Memory.NextGC}} next GC</div><div><b>GC</b><br>{{.Memory.NumGC}} cycles</div>
<div><b>Pool estimate</b><br>{{.Pool.BeforeNextGC}} before GC / {{.Pool.WithinMemoryLimit}} within limit</div></div>
<p>PID {{.PID}} · {{.GoVersion}} · GOMAXPROCS {{.GOMAXPROCS}} · {{.Goroutines}} goroutines</p>
<nav class="tabs"><a href="/debug/server-status/live"{{if eq .View "live"}} class="active"{{end}}>Processes</a><a href="/debug/server-status/log"{{if eq .View "log"}} class="active"{{end}}>Log</a><a href="/debug/server-status/stats"{{if eq .View "stats"}} class="active"{{end}}>Statistics</a></nav>
{{if eq .View "live"}}<section><table><thead><tr><th>S</th><th>Request ID</th><th>Request</th><th>Hostname</th><th>Filename</th><th>Included</th><th>HTTP</th><th>Duration</th><th>Bytes</th><th>Heap Δ</th><th>Allocated</th><th>Objects</th><th>GC / pause</th><th>Remote</th></tr></thead><tbody>
{{range .Requests}}<tr><td><code>{{.Status}}</code></td><td>{{.ID}}</td><td>{{.Request}}</td><td>{{.Hostname}}</td><td>{{.Filename}}</td><td>{{included .IncludedFiles}}</td><td>{{.ResponseStatus}}</td><td>{{duration .Duration}}</td><td>{{.ResponseBytes}}</td><td>{{bytes .HeapDelta}}</td><td>{{bytes .AllocatedBytes}}</td><td>{{.Allocations}}</td><td>{{.GCCycles}} / {{duration .GCPause}}</td><td>{{.RemoteAddress}}</td></tr>{{else}}<tr><td colspan="14">No requests in flight.</td></tr>{{end}}
</tbody></table><p><small>States: _ waiting, s starting, R reading, P processing, W writing, K keepalive, C closing, E error.</small></p>
<h2>Lifetime request state time</h2><div class="state-bar" aria-label="Lifetime request state time">{{range .StateTime}}<span class="{{stateClass .Label}}" style="{{stateWidth .Share}}" title="{{.Label}}: {{duration .Duration}} ({{printf "%.2f" .Share}}%)"></span>{{end}}</div>
<div class="state-legend">{{range .StateTime}}<span><i class="{{stateClass .Label}}"></i><b>{{.Label}} ({{.State}})</b> {{duration .Duration}} · {{printf "%.2f" .Share}}%</span>{{else}}<span>No request state time observed.</span>{{end}}</div>
<p><small>Aggregated across all requests since server start. Concurrent request time is cumulative and may exceed uptime.</small></p></section>{{end}}
{{if eq .View "log"}}<section><nav class="tabs"><a href="/debug/server-status/log?limit=20"{{if eq .Limit 20}} class="active"{{end}}>20</a><a href="/debug/server-status/log?limit=50"{{if eq .Limit 50}} class="active"{{end}}>50</a><a href="/debug/server-status/log?limit=100"{{if eq .Limit 100}} class="active"{{end}}>100</a></nav><table><thead><tr><th>Request ID</th><th>Request</th><th>Hostname</th><th>Filename</th><th>Included</th><th>HTTP</th><th>Duration</th><th>Bytes</th><th>Heap Δ</th><th>Allocated</th><th>Objects</th><th>GC / pause</th><th>Remote</th></tr></thead><tbody>
{{range .Log}}<tr><td>{{.ID}}</td><td>{{.Request}}</td><td>{{.Hostname}}</td><td>{{.Filename}}</td><td>{{included .IncludedFiles}}</td><td>{{.ResponseStatus}}</td><td>{{duration .Duration}}</td><td>{{.ResponseBytes}}</td><td>{{bytes .HeapDelta}}</td><td>{{bytes .AllocatedBytes}}</td><td>{{.Allocations}}</td><td>{{.GCCycles}} / {{duration .GCPause}}</td><td>{{.RemoteAddress}}</td></tr>{{else}}<tr><td colspan="13">No completed requests in the rolling log.</td></tr>{{end}}
</tbody></table></section>{{end}}
{{if eq .View "stats"}}<section><p>Top {{.Statistics.TopLimit}} requests from the rolling window of {{.Statistics.WindowSize}} / {{.Statistics.WindowLimit}} completed requests.</p><table><thead><tr><th>Share</th><th>Count</th><th>Request</th><th>Hostname</th><th>Filename</th><th>Average included</th><th>Average time</th><th>Average bytes</th><th>Average allocated</th></tr></thead><tbody>
{{range .Statistics.Top}}<tr><td class="share"><meter min="0" max="100" value="{{printf "%.2f" .Share}}"></meter>{{printf "%.2f" .Share}}%</td><td>{{.Count}}</td><td>{{.Request}}</td><td>{{.Hostname}}</td><td>{{.Filename}}</td><td>{{averageIncluded .AverageIncludedFiles}}</td><td>{{duration .AverageDuration}}</td><td>{{bytes .AverageResponseBytes}}</td><td>{{bytes .AverageAllocatedBytes}}</td></tr>{{else}}<tr><td colspan="9">No completed requests in the rolling window.</td></tr>{{end}}
</tbody></table></section>{{end}}
<p><small>Per-request memory deltas are process-wide and may overlap when requests run concurrently.</small></p>
</body></html>`))

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
